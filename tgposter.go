// log( :328 :214 :199 :347 :166 :171 :484 :459 :511
/*
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
	"slices"
	"strings"
	"strconv"
	"syscall"
	"time"

	yaml "github.com/goccy/go-yaml"

	"github.com/shoce/tg"
)

const (
	N = ""
	SP = " "
	NL = "\n"
)

var (
	Config TgPosterConfig
	
	Ctx context.Context
	HttpClient = &http.Client{}
	
	ABookOfDaysRe *regexp.Regexp
	ACourseInMiraclesWorkbookRe *regexp.Regexp
	
	F = fmt.Sprintf
	EF = fmt.Errorf
	FI = strconv.FormatInt
	pout = fmt.Print
)

type TgPosterConfig struct {
	YssUrl string `yaml:"-"`
	
	DEBUG bool `yaml:"DEBUG"`
	
	Interval time.Duration `yaml:"Interval"`
	
	TgApiUrlBase string `yaml:"TgApiUrlBase"` // "https://api.telegram.org"
	
	TgToken string `yaml:"TgToken"`
	TgUpdateLog []int64 `yaml:"TgUpdateLog,flow"`
	TgUpdateLogMaxSize int `yaml:"TgUpdateLogMaxSize"` // 333
	
	TgChatId string `yaml:"TgChatId"`
	PostingStartHour int `yaml:"PostingStartHour"`
	
	ABookOfDaysPath     string `yaml:"ABookOfDaysPath"`
	ABookOfDaysTgChatId string `yaml:"ABookOfDaysTgChatId"`
	ABookOfDaysLast     string `yaml:"ABookOfDaysLast"`
	ABookOfDaysReTemplate string `yaml:"ABookOfDaysReTemplate"`
	
	ACourseInMiraclesWorkbookPath     string `yaml:"ACourseInMiraclesWorkbookPath"`
	ACourseInMiraclesWorkbookTgChatId string `yaml:"ACourseInMiraclesWorkbookTgChatId"`
	ACourseInMiraclesWorkbookLast     string `yaml:"ACourseInMiraclesWorkbookLast"`
	ACourseInMiraclesWorkbookReString string `yaml:"ACourseInMiraclesWorkbookReString"`
	
	Chats []TgPosterConfigChat `yaml:"Chats"`
}

type TgPosterConfigChat struct {
	TgChatId string `yaml:"TgChatId"`
	DaysOffset uint `yaml:"DaysOffset"` // days from mar/1
	ABookOfDaysEnabled bool `yaml:"ABookOfDaysEnabled"`
	ABookOfDaysLast string `yaml:"ABookOfDaysLast"`
	ACourseInMiraclesWorkbookEnabled bool `yaml:"ACourseInMiraclesWorkbookEnabled"`
	ACourseInMiraclesWorkbookLast string `yaml:"ACourseInMiraclesWorkbookLast"`
}

func init() {
	var err error
	
	Ctx = context.TODO()
	
	if s := os.Getenv("YssUrl"); s != "" {
		Config.YssUrl = s
	}
	if Config.YssUrl == "" {
		perr("ERROR YssUrl empty")
		os.Exit(1)
	}
	
	if err := Config.Get(); err != nil {
		perr(F("ERROR Config.Get %v", err))
		os.Exit(1)
	}
	
	if Config.DEBUG {
		perr("DEBUG <true>")
	}
	
	perr(F("Interval <%v>", Config.Interval))
	if Config.Interval == 0 {
		perr("ERROR Interval empty")
		os.Exit(1)
	}
	
	if Config.TgToken == "" {
		perr("ERROR TgToken empty")
		os.Exit(1)
	}
	
	tg.ApiToken = Config.TgToken
	
	if Config.TgUpdateLogMaxSize <= 0 {
		Config.TgUpdateLogMaxSize = 333
	}
	
	if Config.TgChatId == "" {
		perr("ERROR TgChatId empty")
		os.Exit(1)
	}
	
	if Config.PostingStartHour < 0 || Config.PostingStartHour > 23 {
		perr(F("ERROR invalid PostingStartHour <%d> must be between <0> and <23>", Config.PostingStartHour))
		os.Exit(1)
	}
	
	if Config.ABookOfDaysReTemplate == "" && Config.ABookOfDaysPath != "" {
		perr("ERROR ABookOfDaysReTemplate is empty")
		os.Exit(1)
	}
	
	if Config.ABookOfDaysTgChatId == "" && Config.ABookOfDaysPath != "" {
		perr("ERROR ABookOfDaysTgChatId is empty")
		os.Exit(1)
	}
	
	if ACourseInMiraclesWorkbookRe, err = regexp.Compile(Config.ACourseInMiraclesWorkbookReString); err != nil {
		perr(F("ERROR invalid ACourseInMiraclesWorkbookReString `%s`: %v", Config.ACourseInMiraclesWorkbookReString, err))
		os.Exit(1)
	}
	if Config.ACourseInMiraclesWorkbookTgChatId == "" && Config.ACourseInMiraclesWorkbookPath != "" {
		perr("ACourseInMiraclesWorkbookTgChatId is empty")
		os.Exit(1)
	}
}

func main() {
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGTERM, syscall.SIGINT)
	go func(sigchan chan os.Signal) {
		sig := <-sigchan
		tglog(F("%s %v", os.Args[0], sig))
		os.Exit(1)
	}(sigchan)

	var chatid string
	var daysoffset uint
	var last string

	var t0 time.Time

	for {

		t0 = time.Now()

		if err := TgGetUpdates(); err != nil {
			perr(F("ERROR TgGetUpdates %v", err))
		}

		chatid = Config.ABookOfDaysTgChatId
		daysoffset = 0
		last = Config.ABookOfDaysLast
		if last2, err := PostABookOfDays(chatid, daysoffset, last); err != nil {
			tglog(F("ERROR PostABookOfDays %v", err))
		} else if last2!="" && last2!=last {
			Config.ABookOfDaysLast = last2
			if err := Config.Put(); err != nil {
				perr(F("ERROR Config.Put %v", err))
			}
		}

		chatid = Config.ACourseInMiraclesWorkbookTgChatId
		last = Config.ACourseInMiraclesWorkbookLast
		if last2, err := PostACourseInMiraclesWorkbook(chatid, daysoffset, last); err != nil {
			tglog(F("ERROR PostACourseInMiraclesWorkbook %v", err))
		} else if last2!="" && last2!=last {
			Config.ACourseInMiraclesWorkbookLast = last2
			if err := Config.Put(); err != nil {
				perr(F("ERROR Config.Put %v", err))
			}
		}

		for ic := range Config.Chats {
			chatid = Config.Chats[ic].TgChatId
			daysoffset = Config.Chats[ic].DaysOffset
			if Config.Chats[ic].ABookOfDaysEnabled {
				last = Config.Chats[ic].ABookOfDaysLast
				if last2, err := PostABookOfDays(chatid, daysoffset, last); err != nil {
					tglog(F("ERROR PostABookOfDays [%s] %v", chatid, err))
				} else if last2!="" && last2!=last {
					Config.Chats[ic].ABookOfDaysLast = last2
					if err := Config.Put(); err != nil {
						perr(F("ERROR Config.Put %v", err))
					}
				}
			}
			if Config.Chats[ic].ACourseInMiraclesWorkbookEnabled {
				last = Config.Chats[ic].ACourseInMiraclesWorkbookLast
				if last2, err := PostACourseInMiraclesWorkbook(chatid, daysoffset, last); err != nil {
					tglog(F("ERROR PostACourseInMiraclesWorkbook [%s] %v", chatid, err))
				} else if last2!="" && last2!=last {
					Config.Chats[ic].ACourseInMiraclesWorkbookLast = last2
					if err := Config.Put(); err != nil {
						perr(F("ERROR Config.Put %v", err))
					}
				}
			}
		}
		
		for time.Now().Sub(t0) < Config.Interval {
			time.Sleep(77*time.Second)
			if err := TgGetUpdates(); err != nil {
				perr(F("ERROR TgGetUpdates %v", err))
			}
		}
		
	}
	
}

func mar1daysoffset(t time.Time) uint {
	// https://pkg.go.dev/time#Time
	y0 := t.Year()
	if t.Month()<3 { y0-- }
	t0 := time.Date(y0, 3, 1, 0, 0, 0, 0, t.Location())
	// https://pkg.go.dev/time#Duration
	return uint(t.Sub(t0).Hours()/24)
}

func PostACourseInMiraclesWorkbook(chatid string, daysoffset uint, last string) (last2 string, err error) {
	if chatid=="" { return }
	if Config.ACourseInMiraclesWorkbookPath == "" { return }
	tnow := time.Now().UTC()
	if tnow.Hour() < Config.PostingStartHour { return }

	if	daysoffset==mar1daysoffset(tnow) {
		if last != "* LESSON 1 *" {
			last = ""
		}
	}

	daynum := mar1daysoffset(tnow) + 1 - daysoffset 
	daynums := F(" %d ", daynum)

	perr(F("DEBUG PostACourseInMiraclesWorkbook daysoffset <%d> daynum <%d>", daysoffset, daynum))

	acimwbbb, err := ioutil.ReadFile(Config.ACourseInMiraclesWorkbookPath)
	if err != nil {
		return "", EF("ReadFile ACourseInMiraclesWorkbookPath [%s] %v", Config.ACourseInMiraclesWorkbookPath, err)
	}
	acimwb := string(acimwbbb)
	if acimwb == "" {
		return "", EF("Empty file ACourseInMiraclesWorkbookPath [%s]", Config.ACourseInMiraclesWorkbookPath)
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
		perr(F("ACourseInMiraclesWorkbook texts len<4000>+ [%s]", strings.Join(longis, "], [")))
	*/

	if strings.Contains(last, daynums) {
		return "", nil
	}

	var skip bool
	if last != "" {
		skip = true
	}

	for _, s := range acimwbss {
		st := strings.Split(s, NL)[0]
		if st == last {
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
			message = tg.EscExcept(message, "*_")
			message = regexp.MustCompile("__+").ReplaceAllStringFunc(message, func(s string) string { return tg.Esc(s) })
			
			perr(F("DEBUG PostACourseInMiraclesWorkbook message [-"+NL+"%s"+NL+"-]", message))
			
			if _, err := tg.SendMessage(tg.SendMessageRequest{
				ChatId: chatid,
				Text:   message,
				
				LinkPreviewOptions: tg.LinkPreviewOptions{IsDisabled: true},
			}); err != nil {
				return "", err
			}
		}
		
		last2 = st
		
		if ACourseInMiraclesWorkbookRe.MatchString(st) {
			break
		}
	}
	
	return last2, nil
}

func PostABookOfDays(chatid string, daysoffset uint, last string) (last2 string, err error) {
	if chatid=="" { return }
	if Config.ABookOfDaysPath == "" { return }
	tnow := time.Now().UTC()
	if tnow.Hour() < Config.PostingStartHour { return }
	
	if Config.ABookOfDaysReTemplate == "" {
		return "", EF("ABookOfDaysReTemplate is empty")
	}
	
	abodbb, err := ioutil.ReadFile(Config.ABookOfDaysPath)
	if err != nil {
		return "", EF("ReadFile ABookOfDaysPath [%s] %v", Config.ABookOfDaysPath, err)
	}
	abod := strings.TrimSpace(string(abodbb))
	if abod == "" {
		return "", EF("Empty file ABookOfDaysPath [%s]", Config.ABookOfDaysPath)
	}
	
	tnow = tnow.Add(time.Duration(daysoffset*24)*time.Hour)
	monthday := tnow.Format("January 2")
	perr(F("DEBUG PostABookOfDays daysoffset <%d> monthday [%s]", daysoffset, monthday))
	
	if monthday == last { return "", nil }
	
	abookofdaysre := strings.ReplaceAll(Config.ABookOfDaysReTemplate, "monthday", monthday)
	perr(F("DEBUG abookofdaysre [%s]", abookofdaysre))
	if ABookOfDaysRe, err = regexp.Compile(abookofdaysre); err != nil {
		return "", err
	}
	abodtoday := ABookOfDaysRe.FindString(abod)
	abodtoday = strings.TrimSpace(abodtoday)
	if abodtoday == "" {
		perr("Could not find A Book of Days text for today")
		return "", nil
	}
	
	abodtoday = tg.EscExcept(abodtoday, "*_")
	
	perr(F("DEBUG abodtoday [-"+NL+"%s"+NL+"-]", abodtoday))
	
	if _, err := tg.SendMessage(tg.SendMessageRequest{
		ChatId: Config.ABookOfDaysTgChatId,
		Text:   abodtoday,
		
		LinkPreviewOptions: tg.LinkPreviewOptions{IsDisabled: true},
	}); err != nil {
		return "", err
	}
	
	last2 = monthday
	if err := Config.Put(); err != nil {
		return "", EF("ERROR Config.Put %w", err)
	}
	
	return last2, nil
}

func TgGetUpdates() (err error) {
	
	var updatesoffset int64
	
	if len(Config.TgUpdateLog) > 0 {
		updatesoffset = Config.TgUpdateLog[len(Config.TgUpdateLog)-1] + 1
	}
	
	var uu []tg.Update
	var tgupdatesjson string
	uu, tgupdatesjson, err = tg.GetUpdates(updatesoffset)
	if err != nil {
		return EF("tg.GetUpdates %v", err)
	}
	
	for _, u := range uu {
		//perr("DEBUG Update" + SP + strings.ReplaceAll(F("%+v", u), NL, "<NL>"))
		/*
			if len(TgUpdateLog)>0 && u.UpdateId<TgUpdateLog[len(TgUpdateLog)-1] {
				perr(F("WARNING this telegram update id <%d> is older than last id <%d>, skipping", u.UpdateId, TgUpdateLog[len(TgUpdateLog)-1]))
				continue
			}
		*/
		if slices.Contains(Config.TgUpdateLog, u.UpdateId) {
			perr(F("WARNING this telegram update id <%d> was already processed, skipping", u.UpdateId))
			continue
		}
		Config.TgUpdateLog = append(Config.TgUpdateLog, u.UpdateId)
		if len(Config.TgUpdateLog) > Config.TgUpdateLogMaxSize {
			Config.TgUpdateLog = Config.TgUpdateLog[len(Config.TgUpdateLog)-Config.TgUpdateLogMaxSize:]
		}
		if err := Config.Put(); err != nil {
			return EF("Config.Put %v", err)
		}
		
		var m tg.Message
		if m, err = processTgUpdate(u, tgupdatesjson); err == nil {
			if tgerr := tg.SetMessageReaction(tg.SetMessageReactionRequest{
				ChatId:    FI(m.Chat.Id, 10),
				MessageId: m.MessageId,
				Reaction:  []tg.ReactionTypeEmoji{tg.ReactionTypeEmoji{Emoji: "👌"}},
			}); tgerr != nil {
				perr(F("ERROR tg.SetMessageReaction [👌] %v", tgerr))
			}
		} else {
			perr(F("ERROR processTgUpdate %v", err))
			if tgerr := tg.SetMessageReaction(tg.SetMessageReactionRequest{
				ChatId:    FI(m.Chat.Id, 10),
				MessageId: m.MessageId,
				Reaction:  []tg.ReactionTypeEmoji{tg.ReactionTypeEmoji{Emoji: "😭"}},
			}); tgerr != nil {
				perr(F("ERROR tg.SetMessageReaction [😭] %v", tgerr))
			}
		}
		if err := Config.Put(); err != nil {
			return EF("Config.Put %v", err)
		}
	}
	
	return nil
	
}

func processTgUpdate(u tg.Update, tgupdatesjson string) (m tg.Message, err error) {
	
	m = u.Message
	chatid := FI(m.From.Id, 10)
	if m.MessageId==0 {
		return m, EF("unknown update")
	}
	perr(F("DEBUG Message %v", m))
	switch m.Text {
		
	case "/start":
		updated := false
		for ic, _ := range Config.Chats {
			if Config.Chats[ic].TgChatId == chatid {
				Config.Chats[ic].DaysOffset = mar1daysoffset(time.Now().UTC())
				//Config.Chats[ic].ABookOfDaysEnabled = true
				//Config.Chats[ic].ABookOfDaysLast = ""
				Config.Chats[ic].ACourseInMiraclesWorkbookEnabled = true
				Config.Chats[ic].ACourseInMiraclesWorkbookLast = ""
				updated = true
			}
		}
		if !updated {
			Config.Chats = append(Config.Chats, TgPosterConfigChat{
				TgChatId: chatid,
				DaysOffset: mar1daysoffset(time.Now().UTC()),
				//ABookOfDaysEnabled: true,
				ACourseInMiraclesWorkbookEnabled: true,
			})
		}
		if err := Config.Put(); err != nil {
			return m, EF("Config.Put %v", err)
		}
		
		tgmsg := (
			"hello, welcome. here you have found *a course in miracles workbook* in form of daily messages. when you start the bot, you start the course from day one. to stop receiving daily messages send `/stop`. send `/start` to restart the course from the beginning."+NL+
			"peace and joy!"+NL+
			"✌️"+NL+
			N)
		if _, err := tg.SendMessage(tg.SendMessageRequest{
			ChatId: Config.TgChatId,
			Text: tgmsg,
			DisableNotification: true,
			LinkPreviewOptions: tg.LinkPreviewOptions{IsDisabled: true},
		}); err!=nil {
			perr(F("ERROR tg.SendMessage %v", err))
		}
	
	case "/stop":
		for ic, _ := range Config.Chats {
			if Config.Chats[ic].TgChatId == chatid {
				Config.Chats[ic].ABookOfDaysEnabled = false
				Config.Chats[ic].ACourseInMiraclesWorkbookEnabled = false
			}
		}
		if err := Config.Put(); err != nil {
			return m, EF("Config.Put %v", err)
		}
		tgmsg := "stopped. to restart send `/start`."+NL
		if _, err := tg.SendMessage(tg.SendMessageRequest{
			ChatId: Config.TgChatId,
			Text: tgmsg,
			DisableNotification: true,
			LinkPreviewOptions: tg.LinkPreviewOptions{IsDisabled: true},
		}); err!=nil {
			perr(F("ERROR tg.SendMessage %v", err))
		}
	
	default:
		return m, EF("unknown command [%s]", m.Text)
	
	}
	
	return
}

func ts() string {
	tnow := time.Now()
	return fmt.Sprintf(
		"%d%02d%02d:%02d%02d-",
		tnow.Year()%1000, tnow.Month(), tnow.Day(),
		tnow.Hour(), tnow.Minute(),
	)
}

func perr(msg string) {
	fmt.Fprint(os.Stderr, ts()+SP+msg+NL)
}

func tglog(msg string) (err error) {
	perr(msg)
	_, err = tg.SendMessage(tg.SendMessageRequest{
		ChatId: Config.TgChatId,
		Text: tg.Esc(msg),
		DisableNotification: true,
		LinkPreviewOptions: tg.LinkPreviewOptions{IsDisabled: true},
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
		return EF("yss response status %s", resp.Status)
	}
	
	rbb, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	
	if err := yaml.Unmarshal(rbb, config); err != nil {
		return err
	}
	
	//perr(F("DEBUG Config.Get [-"+NL+"%s"+NL+"-]", rbb))
	//perr(F("DEBUG Config.Get %+v", config))
	
	return nil
}

func (config *TgPosterConfig) Put() error {
	//perr(F("DEBUG Config.Put %s %+v", config.YssUrl, config))
	
	// https://pkg.go.dev/github.com/goccy/go-yaml#MarshalWithOptions
	rbb, err := yaml.MarshalWithOptions(config, yaml.JSON(), yaml.Flow(false))
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
		return EF("yss response status %s", resp.Status)
	}
	
	return nil
}


