// log( :328 :214 :199 :347 :166 :171 :484
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
	"syscall"
	"time"

	yaml "gopkg.in/yaml.v3"

	"github.com/shoce/tg"
)

const (
	SP = " "
	NL = "\n"
)

type TgPosterConfig struct {
	YssUrl string `yaml:"-"`
	
	DEBUG bool
	
	Interval time.Duration
	
	TgApiUrlBase string // "https://api.telegram.org"
	
	TgToken  string
	TgUpdateLog []int64
	TgUpdateLogMaxSize int // 333
	
	TgChatId string
	PostingStartHour int
	
	ABookOfDaysPath     string
	ABookOfDaysTgChatId string
	ABookOfDaysLast     string
	ABookOfDaysReTemplate string
	
	ACourseInMiraclesWorkbookPath     string
	ACourseInMiraclesWorkbookTgChatId string
	ACourseInMiraclesWorkbookLast     string
	ACourseInMiraclesWorkbookReString string
	
	Chats []TgPosterConfigChat
}

type TgPosterConfigChat struct {
	TgChatId string
	DaysOffset uint // days from mar/1
	ABookOfDaysEnabled bool
	ABookOfDaysLast string
	ACourseInMiraclesWorkbookEnabled bool
	ACourseInMiraclesWorkbookLast string
}

var (
	Config TgPosterConfig
	
	Ctx context.Context
	HttpClient = &http.Client{}
	
	ABookOfDaysRe *regexp.Regexp
	ACourseInMiraclesWorkbookRe *regexp.Regexp
	
	F = fmt.Sprintf
	EF = fmt.Errorf
	pout = fmt.Print
)

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
			time.Sleep(111*time.Second)
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
	t0 := time.Date(y0, t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
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

	perr(F("DEBUG daynum <%v>", daynum))

	acimwbbb, err := ioutil.ReadFile(Config.ACourseInMiraclesWorkbookPath)
	if err != nil {
		return "", EF("ReadFile ACourseInMiraclesWorkbookPath %s %v", Config.ACourseInMiraclesWorkbookPath, err)
	}
	acimwb := string(acimwbbb)
	if acimwb == "" {
		return "", EF("Empty file ACourseInMiraclesWorkbookPath %s", Config.ACourseInMiraclesWorkbookPath)
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

			if Config.DEBUG {
				perr(F("DEBUG message [%s]", message))
			}

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
	perr(F("DEBUG monthday [%s]", monthday))

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
		perr("Update" + SP + strings.ReplaceAll(F("%+v", u), NL, "<NL>"))
		/*
			if len(TgUpdateLog) > 0 && u.UpdateId < TgUpdateLog[len(TgUpdateLog)-1] {
				log("WARNING this telegram update id <%d> is older than last id <%d>, skipping", u.UpdateId, TgUpdateLog[len(TgUpdateLog)-1])
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
		
		if m, err := processTgUpdate(u, tgupdatesjson); err != nil {
			perr(F("ERROR processTgUpdate %v", err))
			if tgerr := tg.SetMessageReaction(tg.SetMessageReactionRequest{
				ChatId:    fmt.Sprintf("%d", m.Chat.Id),
				MessageId: m.MessageId,
				Reaction:  []tg.ReactionTypeEmoji{tg.ReactionTypeEmoji{Emoji: "🤷‍♂"}},
			}); tgerr != nil {
				perr(F("ERROR tg.SetMessageReaction [🤷‍♂] %v", tgerr))
			}
			return err
		}
		if err := Config.Put(); err != nil {
			return EF("Config.Put %v", err)
		}
	}
	
	return nil
	
}

func processTgUpdate(u tg.Update, tgupdatesjson string) (m tg.Message, err error) {
	
	cmu := u.MyChatMember
	if cmu.Date!=0 && cmu.NewChatMember.Status=="member" {
		cmuid := F("%d", cmu.NewChatMember.User.Id)
		updated := false
		for ic, _ := range Config.Chats {
			if Config.Chats[ic].TgChatId == cmuid {
				Config.Chats[ic].DaysOffset = mar1daysoffset(time.Now().UTC())
				//Config.Chats[ic].ABookOfDaysEnabled = true
				Config.Chats[ic].ACourseInMiraclesWorkbookEnabled = true
				updated = true
			}
		}
		if !updated {
			Config.Chats = append(Config.Chats, TgPosterConfigChat{
				TgChatId: cmuid,
				DaysOffset: mar1daysoffset(time.Now().UTC()),
				//ABookOfDaysEnabled: true,
				ACourseInMiraclesWorkbookEnabled: true,
			})
		}
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
		Text:   tg.Esc(msg),

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
		return EF("yss response status %s", resp.Status)
	}

	rbb, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(rbb, config); err != nil {
		return err
	}

	if config.DEBUG {
		perr(F("DEBUG Config.Get %+v", config))
	}

	return nil
}

func (config *TgPosterConfig) Put() error {
	if config.DEBUG {
		perr(F("DEBUG Config.Put %s %+v", config.YssUrl, config))
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
		return EF("yss response status %s", resp.Status)
	}

	return nil
}


