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

	"github.com/shoce/tg"
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

	tg.ApiToken = Config.TgToken

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
		tglog("%s: sigterm", os.Args[0])
		os.Exit(1)
	}(sigterm)

	for {
		t0 := time.Now()

		err = PostMoonPhaseToday()
		if err != nil {
			log("ERROR PostMoonPhaseToday: %v", err)
		}

		err = PostABookOfDays()
		if err != nil {
			log("ERROR PostABookOfDays: %v", err)
		}

		err = PostACourseInMiraclesWorkbook()
		if err != nil {
			log("ERROR PostACourseInMiraclesWorkbook: %v", err)
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
	daynum := int(time.Since(ty0)/(24*time.Hour) + 1)
	daynums := fmt.Sprintf(" %d ", daynum)

	if Config.DEBUG {
		log("DEBUG daynum==%v", daynum)
	}

	acimwbbb, err := ioutil.ReadFile(Config.ACourseInMiraclesWorkbookPath)
	if err != nil {
		return fmt.Errorf("ReadFile ACourseInMiraclesWorkbookPath=`%s`: %v", Config.ACourseInMiraclesWorkbookPath, err)
	}
	acimwb := string(acimwbbb)
	if acimwb == "" {
		return fmt.Errorf("Empty file ACourseInMiraclesWorkbookPath=`%s`", Config.ACourseInMiraclesWorkbookPath)
	}
	acimwbss := strings.Split(acimwb, NL+NL+NL+NL)

	/*
		var longis []string
		for _, t := range acimwbss {
			if len(t) >= 4000 {
				tt := strings.Split(t, NL)[0]
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
		st := strings.Split(s, NL)[0]
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
			srs := strings.Split(s, NL+NL)
			for i, s := range srs {
				sp += s + NL + NL
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

			// https://pkg.go.dev/regexp#Regexp.ReplaceAllStringFunc
			message = regexp.MustCompile("__+").ReplaceAllStringFunc(message, tg.Esc)
			message = tg.EscBI(message)

			if Config.DEBUG {
				log("DEBUG message==%v", message)
			}

			if _, err := tg.SendMessage(tg.SendMessageRequest{
				ChatId: Config.ACourseInMiraclesWorkbookTgChatId,
				Text:   message,

				LinkPreviewOptions: tg.LinkPreviewOptions{IsDisabled: true},
			}); err != nil {
				return err
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

	abodtoday = tg.EscBI(abodtoday)

	if Config.DEBUG {
		log("DEBUG abodtoday:"+NL+"%s", abodtoday)
	}

	if _, err := tg.SendMessage(tg.SendMessageRequest{
		ChatId: Config.ABookOfDaysTgChatId,
		Text:   abodtoday,

		LinkPreviewOptions: tg.LinkPreviewOptions{IsDisabled: true},
	}); err != nil {
		return err
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

	message := tg.Esc(moonphase)

	if moonphase != "" {
		if _, err := tg.SendMessage(tg.SendMessageRequest{
			ChatId: Config.MoonPhaseTgChatId,
			Text:   message,

			LinkPreviewOptions: tg.LinkPreviewOptions{IsDisabled: true},
		}); err != nil {
			return err
		}
	}

	Config.MoonPhaseTodayLast = yearmonthday
	err = Config.Put()
	if err != nil {
		return fmt.Errorf("ERROR Config.Put: %v", err)
	}

	return nil
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

func ts() string {
	tnow := time.Now().Local()
	return fmt.Sprintf(
		"%d%02d%02d:%02d%02d±",
		tnow.Year()%1000, tnow.Month(), tnow.Day(),
		tnow.Hour(), tnow.Minute(),
	)
}

func log(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, ts()+" "+msg+NL, args...)
}

func tglog(msg string, args ...interface{}) (err error) {
	log(msg, args...)
	text := fmt.Sprintf(msg, args...) + NL
	text = tg.Esc(text)
	_, err = tg.SendMessage(tg.SendMessageRequest{
		ChatId: Config.TgChatId,
		Text:   text,

		DisableNotification: true,
		LinkPreviewOptions:  tg.LinkPreviewOptions{IsDisabled: true},
	})
	return err
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
