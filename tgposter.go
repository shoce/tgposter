/*

https://core.telegram.org/bots/api/
https://core.telegram.org/bots/api/#formatting-options

GoGet
GoFmt
GoBuildNull

*/

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	yaml "gopkg.in/yaml.v3"
)

const (
	NL = "\n"
)

type TgPosterConfig struct {
	YssUrl string `yaml:"-"`

	DEBUG bool `yaml:"DEBUG"`

	Interval time.Duration `yaml:"Interval"`

	TgToken  string `yaml:"TgToken"`
	TgChatId string `yaml:"TgChatId"`

	PostingStartHour int `yaml:"PostingStartHour"`

	MoonPhaseTgChatId  string `yaml:"MoonPhaseTgChatId"`
	MoonPhaseTodayLast string `yaml:"MoonPhaseTodayLast"`

	ABookOfDaysPath     string `yaml:"ABookOfDaysPath"`
	ABookOfDaysLast     string `yaml:"ABookOfDaysLast"`
	ABookOfDaysTgChatId string `yaml:"ABookOfDaysTgChatId"`

	ABookOfDaysReTemplate string `yaml:"ABookOfDaysReTemplate"`

	ACourseInMiraclesWorkbookPath     string `yaml:"ACourseInMiraclesWorkbookPath"`
	ACourseInMiraclesWorkbookLast     string `yaml:"ACourseInMiraclesWorkbookLast"`
	ACourseInMiraclesWorkbookTgChatId string `yaml:"ACourseInMiraclesWorkbookTgChatId"`
	ACourseInMiraclesWorkbookReString string `yaml:"ACourseInMiraclesWorkbookReString"`
}

var (
	Config TgPosterConfig

	Ctx context.Context

	HttpClient = &http.Client{}

	ABookOfDaysRe               *regexp.Regexp
	ACourseInMiraclesWorkbookRe *regexp.Regexp
)

func init() {
	var err error

	Ctx = context.TODO()

	if s := os.Getenv("YssUrl"); s != "" {
		Config.YssUrl = s
	}
	if Config.YssUrl == "" {
		log("ERROR YssUrl empty")
		os.Exit(1)
	}

	if err := Config.Get(); err != nil {
		log("ERROR Config.Get: %v", err)
		os.Exit(1)
	}

	if Config.DEBUG {
		log("DEBUG==true")
	}

	log("Interval: %v", Config.Interval)
	if Config.Interval == 0 {
		log("ERROR Interval empty")
		os.Exit(1)
	}

	if Config.TgToken == "" {
		log("ERROR TgToken empty")
		os.Exit(1)
	}

	if Config.TgChatId == "" {
		log("ERROR TgChatId empty")
		os.Exit(1)
	}

	if Config.PostingStartHour < 0 || Config.PostingStartHour > 23 {
		log("ERROR invalid PostingStartHour %d: must be between 0 and 23", Config.PostingStartHour)
		os.Exit(1)
	}

	if Config.MoonPhaseTgChatId == "" {
		Config.MoonPhaseTgChatId = Config.TgChatId
	}

	if Config.ABookOfDaysReTemplate == "" && Config.ABookOfDaysPath != "" {
		log("ERROR ABookOfDaysReTemplate is empty")
		os.Exit(1)
	}

	if Config.ABookOfDaysTgChatId == "" && Config.ABookOfDaysPath != "" {
		log("ERROR ABookOfDaysTgChatId is empty")
		os.Exit(1)
	}

	if ACourseInMiraclesWorkbookRe, err = regexp.Compile(Config.ACourseInMiraclesWorkbookReString); err != nil {
		log("ERROR invalid ACourseInMiraclesWorkbookReString `%s`: %v", Config.ACourseInMiraclesWorkbookReString, err)
		os.Exit(1)
	}
	if Config.ACourseInMiraclesWorkbookTgChatId == "" && Config.ACourseInMiraclesWorkbookPath != "" {
		log("ACourseInMiraclesWorkbookTgChatId is empty")
		os.Exit(1)
	}
}

func main() {
	var err error

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)
	go func(sigterm chan os.Signal) {
		<-sigterm
		tgsendMessage(fmt.Sprintf("%s: sigterm", os.Args[0]), Config.TgChatId, "")
		log("sigterm received")
		os.Exit(1)
	}(sigterm)

	for {
		t0 := time.Now()

		err = PostMoonPhaseToday()
		if err != nil {
			log("WARNING PostMoonPhaseToday: %v", err)
		}

		err = PostABookOfDays()
		if err != nil {
			log("WARNING PostABookOfDays: %v", err)
		}

		err = PostACourseInMiraclesWorkbook()
		if err != nil {
			log("WARNING PostACourseInMiraclesWorkbook: %v", err)
		}

		if dur := time.Now().Sub(t0); dur < Config.Interval {
			time.Sleep(Config.Interval - dur)
		}
	}

}

func PostACourseInMiraclesWorkbook() error {
	if Config.ACourseInMiraclesWorkbookPath == "" || time.Now().UTC().Hour() < Config.PostingStartHour {
		return nil
	}

	if time.Now().UTC().Month() == 3 && time.Now().UTC().Day() == 1 && Config.ACourseInMiraclesWorkbookLast != "* LESSON 1 *" {
		Config.ACourseInMiraclesWorkbookLast = ""
	}

	var ty0 time.Time
	if time.Now().UTC().Month() < 3 {
		ty0 = time.Date(time.Now().UTC().Year()-1, time.Month(3), 1, 0, 0, 0, 0, time.UTC)
	} else {
		ty0 = time.Date(time.Now().UTC().Year(), time.Month(3), 1, 0, 0, 0, 0, time.UTC)
	}
	daynum := time.Since(ty0)/(24*time.Hour) + 1
	daynums := fmt.Sprintf(" %d ", daynum)

	acimwbbb, err := ioutil.ReadFile(Config.ACourseInMiraclesWorkbookPath)
	if err != nil {
		return fmt.Errorf("ReadFile ACourseInMiraclesWorkbookPath=`%s`: %v", Config.ACourseInMiraclesWorkbookPath, err)
	}
	acimwb := string(acimwbbb)
	if acimwb == "" {
		return fmt.Errorf("Empty file ACourseInMiraclesWorkbookPath=`%s`", Config.ACourseInMiraclesWorkbookPath)
	}
	acimwbss := strings.Split(acimwb, "\n\n\n\n")

	/*
		var longis []string
		for _, t := range acimwbss {
			if len(t) >= 4000 {
				tt := strings.Split(t, "\n")[0]
				longis = append(longis, tt)
			}
		}
		log("ACourseInMiraclesWorkbook texts of 4000+ length: %s", strings.Join(longis, ", "))
	*/

	if strings.Contains(Config.ACourseInMiraclesWorkbookLast, daynums) {
		return nil
	}

	var skip bool
	if Config.ACourseInMiraclesWorkbookLast != "" {
		skip = true
	}

	for _, s := range acimwbss {
		st := strings.Split(s, "\n")[0]
		if st == Config.ACourseInMiraclesWorkbookLast {
			skip = false
			continue
		}
		if skip {
			continue
		}

		var spp []string
		if len(s) < 4000 {
			spp = append(spp, s)
		} else {
			var sp string
			srs := strings.Split(s, "\n\n")
			for i, s := range srs {
				sp += s + "\n\n"
				if i == len(srs)-1 || len(sp)+len(srs[i+1]) > 4000 {
					spp = append(spp, sp)
					sp = ""
				}
			}
		}

		for i, sp := range spp {
			message := sp
			if i > 0 {
				message = st + " (continued)\n\n" + sp
			}
			_, err = tgsendMessage(message, Config.ACourseInMiraclesWorkbookTgChatId, "MarkdownV2")
			if err != nil {
				return fmt.Errorf("tgsendMessage: %v", err)
			}
		}

		Config.ACourseInMiraclesWorkbookLast = st

		err = Config.Put()
		if err != nil {
			return fmt.Errorf("ERROR Config.Put: %v", err)
		}

		if ACourseInMiraclesWorkbookRe.MatchString(st) {
			break
		}
	}

	return nil
}

func PostABookOfDays() error {
	if Config.ABookOfDaysPath == "" || time.Now().UTC().Hour() < Config.PostingStartHour {
		return nil
	}

	if Config.ABookOfDaysReTemplate == "" {
		return fmt.Errorf("ABookOfDaysReTemplate is empty")
	}

	abodbb, err := ioutil.ReadFile(Config.ABookOfDaysPath)
	if err != nil {
		return fmt.Errorf("ReadFile ABookOfDaysPath `%s`: %v", Config.ABookOfDaysPath, err)
	}
	abod := strings.TrimSpace(string(abodbb))
	if abod == "" {
		return fmt.Errorf("Empty file ABookOfDaysPath `%s`", Config.ABookOfDaysPath)
	}

	monthday := time.Now().UTC().Format("January 2")
	if Config.DEBUG {
		log("DEBUG monthday:`%s`", monthday)
	}

	if monthday == Config.ABookOfDaysLast {
		return nil
	}

	abookofdaysre := strings.ReplaceAll(Config.ABookOfDaysReTemplate, "monthday", monthday)
	if Config.DEBUG {
		log("DEBUG abookofdaysre:`%s`", abookofdaysre)
	}
	if ABookOfDaysRe, err = regexp.Compile(abookofdaysre); err != nil {
		return err
	}
	abodtoday := ABookOfDaysRe.FindString(abod)
	abodtoday = strings.TrimSpace(abodtoday)
	if abodtoday == "" {
		log("Could not find A Book of Days text for today")
		return nil
	}

	if Config.DEBUG {
		log("DEBUG abodtoday:"+NL+"%s", abodtoday)
	}

	_, err = tgsendMessage(abodtoday, Config.ABookOfDaysTgChatId, "MarkdownV2")
	if err != nil {
		return fmt.Errorf("tgsendMessage: %w", err)
	}

	Config.ABookOfDaysLast = monthday
	err = Config.Put()
	if err != nil {
		return fmt.Errorf("ERROR Config.Put: %w", err)
	}

	return nil
}

func PostMoonPhaseToday() error {
	var err error

	if time.Now().UTC().Hour() < Config.PostingStartHour {
		return nil
	}

	moonphase := MoonPhaseToday()

	yearmonthday := time.Now().UTC().Format("2006 January 2")
	if yearmonthday == Config.MoonPhaseTodayLast {
		return nil
	}

	if moonphase != "" {
		_, err = tgsendMessage(moonphase, Config.MoonPhaseTgChatId, "MarkdownV2")
		if err != nil {
			return fmt.Errorf("tgsendMessage: %v", err)
		}
	}

	Config.MoonPhaseTodayLast = yearmonthday
	err = Config.Put()
	if err != nil {
		return fmt.Errorf("ERROR Config.Put: %v", err)
	}

	return nil
}

func ts() string {
	t := time.Now().Local()
	return fmt.Sprintf(
		"%03d."+"%02d%02d."+"%02d%02d",
		t.Year()%1000, t.Month(), t.Day(), t.Hour(), t.Minute(),
	)
}

func log(msg interface{}, args ...interface{}) {
	msgtext := fmt.Sprintf("%s %s", ts(), msg) + NL
	fmt.Fprintf(os.Stderr, msgtext, args...)
}

func MoonPhaseCalendar() string {
	nmfm := []string{"○", "●"}
	const MoonCycleDur time.Duration = 2551443 * time.Second
	var NewMoon time.Time = time.Date(2020, time.December, 14, 16, 16, 0, 0, time.UTC)
	var sinceNM time.Duration = time.Since(NewMoon) % MoonCycleDur
	var lastNM time.Time = time.Now().UTC().Add(-sinceNM)
	var msg, year, month string
	var mo time.Time = lastNM
	for i := 0; mo.Before(lastNM.Add(time.Hour * 24 * 7 * 54)); i++ {
		if mo.Format("2006") != year {
			year = mo.Format("2006")
			msg += NL + NL + fmt.Sprintf("Year %s", year) + NL
		}
		if mo.Format("Jan") != month {
			month = mo.Format("Jan")
			msg += NL + fmt.Sprintf("%s ", month)
		}
		msg += fmt.Sprintf(
			"%s:%s ",
			mo.Add(-4*time.Hour).Format("Mon/2"),
			nmfm[i%2],
		)
		mo = mo.Add(MoonCycleDur / 2)
	}
	return msg
}

func MoonPhaseToday() string {
	const MoonCycleDur time.Duration = 2551443 * time.Second
	var NewMoon time.Time = time.Date(2020, time.December, 14, 16, 16, 0, 0, time.UTC)
	var sinceNew time.Duration = time.Since(NewMoon) % MoonCycleDur
	var tnow time.Time = time.Now().UTC()
	if tillNew := MoonCycleDur - sinceNew; tillNew < 24*time.Hour {
		return fmt.Sprintf(
			"Today %s is New Moon; next Full Moon is on %s.",
			tnow.Format("Monday, January 2"),
			tnow.Add(MoonCycleDur/2).Format("Monday, January 2"),
		)
	}
	if tillFull := MoonCycleDur/2 - sinceNew; tillFull >= 0 && tillFull < 24*time.Hour {
		return fmt.Sprintf(
			"Today %s is Full Moon; next New Moon is on %s.",
			tnow.Format("Monday, January 2"),
			tnow.Add(MoonCycleDur/2).Format("Monday, January 2"),
		)
	}
	return ""
}

type TgResponse struct {
	Ok          bool       `json:"ok"`
	Description string     `json:"description"`
	Result      *TgMessage `json:"result"`
}

type TgResponseShort struct {
	Ok          bool   `json:"ok"`
	Description string `json:"description"`
}

type TgMessage struct {
	MessageId int64 `json:"message_id"`
	Text      string
}

type TgSendMessageRequest struct {
	ChatId                string `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode,omitempty"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview"`
	DisableNotification   bool   `json:"disable_notification"`
}

func tgsendMessage(text string, chatid string, parsemode string) (msg *TgMessage, err error) {
	// https://core.telegram.org/bots/api/#sendmessage
	// https://core.telegram.org/bots/api/#formatting-options
	if parsemode == "MarkdownV2" {
		for _, c := range []string{`[`, `]`, `(`, `)`, `~`, "`", `>`, `#`, `+`, `-`, `=`, `|`, `{`, `}`, `.`, `!`} {
			text = strings.ReplaceAll(text, c, `\`+c)
		}
		text = strings.ReplaceAll(text, "______", `\_\_\_\_\_\_`)
		text = strings.ReplaceAll(text, "_____", `\_\_\_\_\_`)
		text = strings.ReplaceAll(text, "____", `\_\_\_\_`)
		text = strings.ReplaceAll(text, "___", `\_\_\_`)
		text = strings.ReplaceAll(text, "__", `\_\_`)
	}
	sendMessage := TgSendMessageRequest{
		ChatId:                chatid,
		Text:                  text,
		ParseMode:             parsemode,
		DisableWebPagePreview: true,
	}
	sendMessageJSON, err := json.Marshal(sendMessage)
	if err != nil {
		return nil, err
	}

	var tgresp TgResponse
	err = postJson(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", Config.TgToken),
		bytes.NewBuffer(sendMessageJSON),
		&tgresp,
	)
	if err != nil {
		return nil, err
	}

	if !tgresp.Ok {
		return nil, fmt.Errorf("sendMessage: %s", tgresp.Description)
	}

	msg = tgresp.Result

	return msg, nil
}

func getJson(url string, target interface{}) error {
	r, err := HttpClient.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	return json.NewDecoder(r.Body).Decode(target)
}

func postJson(url string, data *bytes.Buffer, target interface{}) error {
	resp, err := HttpClient.Post(
		url,
		"application/json",
		data,
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody := bytes.NewBuffer(nil)
	_, err = io.Copy(respBody, resp.Body)
	if err != nil {
		return fmt.Errorf("io.Copy: %v", err)
	}

	err = json.NewDecoder(respBody).Decode(target)
	if err != nil {
		return fmt.Errorf("Decode: %v", err)
	}

	return nil
}

func (config *TgPosterConfig) Get() error {
	req, err := http.NewRequest(http.MethodGet, config.YssUrl, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("yss response status %s", resp.Status)
	}

	rbb, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(rbb, config); err != nil {
		return err
	}

	if config.DEBUG {
		log("DEBUG Config.Get: %+v", config)
	}

	return nil
}

func (config *TgPosterConfig) Put() error {
	if config.DEBUG {
		log("DEBUG Config.Put %s %+v", config.YssUrl, config)
	}

	rbb, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, config.YssUrl, bytes.NewBuffer(rbb))
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("yss response status %s", resp.Status)
	}

	return nil
}
