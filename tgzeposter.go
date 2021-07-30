/*
https://core.telegram.org/bots/api/
https://core.telegram.org/bots/api/#formatting-options

go mod init github.com/shoce/tgzeposter
go get -a -u -v
go mod tidy

GoFmt GoBuildNull GoPublish

*/

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	NL = "\n"
)

var (
	YamlConfigPath = "tgzeposter.yaml"

	KvToken       string
	KvAccountId   string
	KvNamespaceId string

	Ctx context.Context

	HttpClient = &http.Client{}

	TgToken    string
	TgZeChatId int64

	MoonPhaseTgChatId  int64
	MoonPhaseTodayLast string

	ABookOfDaysPath     string
	ABookOfDaysLast     string
	ABookOfDaysTgChatId int64
	ABookOfDaysRe       *regexp.Regexp

	ACourseInMiraclesWorkbookPath     string
	ACourseInMiraclesWorkbookLast     string
	ACourseInMiraclesWorkbookTgChatId int64
	ACourseInMiraclesWorkbookReString = "^\\* LESSON "
	ACourseInMiraclesWorkbookRe       *regexp.Regexp
)

func log(msg interface{}, args ...interface{}) {
	t := time.Now().Local()
	ts := fmt.Sprintf(
		"%03dy."+"%02d%02dd."+"%02dh"+"%02dm.",
		t.Year()%1000, t.Month(), t.Day(), t.Hour(), t.Minute(),
	)
	msgtext := fmt.Sprintf("%s %s", ts, msg) + NL
	fmt.Fprintf(os.Stderr, msgtext, args...)
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

func GetVar(name string) (value string) {
	//log("DEBUG GetVar: %s", name)

	var err error

	value = os.Getenv(name)
	if value != "" {
		return value
	}

	if YamlConfigPath != "" {
		value, err = YamlGet(name)
		if err != nil {
			log("WARNING GetVar YamlGet %s: %v", name, err)
		}
		if value != "" {
			return value
		}
	}

	if KvToken != "" && KvAccountId != "" && KvNamespaceId != "" {
		if v, err := KvGet(name); err != nil {
			log("WARNING GetVar KvGet %s: %v", name, err)
			return ""
		} else {
			value = v
		}
	}

	return value
}

func SetVar(name, value string) (err error) {
	//log("DEBUG SetVar: %s: %s", name, value)

	if KvToken != "" && KvAccountId != "" && KvNamespaceId != "" {
		err = KvSet(name, value)
		if err != nil {
			return err
		}
		return nil
	}

	if YamlConfigPath != "" {
		err = YamlSet(name, value)
		if err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("not kv credentials nor yaml config path provided to save to")
}

func YamlGet(name string) (value string, err error) {
	configf, err := os.Open(YamlConfigPath)
	if err != nil {
		//log("WARNING: os.Open config file %s: %v", YamlConfigPath, err)
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer configf.Close()

	configm := make(map[interface{}]interface{})
	if err = yaml.NewDecoder(configf).Decode(&configm); err != nil {
		//log("WARNING: yaml.Decode %s: %v", YamlConfigPath, err)
		return "", err
	}

	if v, ok := configm[name]; ok == true {
		switch v.(type) {
		case string:
			value = v.(string)
		case int:
			value = fmt.Sprintf("%d", v.(int))
		default:
			return "", fmt.Errorf("yaml value of unsupported type, only string and int types are supported")
		}
	}

	return value, nil
}

func YamlSet(name, value string) error {
	configf, err := os.Open(YamlConfigPath)
	if err == nil {
		configm := make(map[interface{}]interface{})
		err := yaml.NewDecoder(configf).Decode(&configm)
		if err != nil {
			log("WARNING: yaml.Decode %s: %v", YamlConfigPath, err)
		}
		configf.Close()
		configm[name] = value
		configf, err := os.Create(YamlConfigPath)
		if err == nil {
			defer configf.Close()
			confige := yaml.NewEncoder(configf)
			err := confige.Encode(configm)
			if err == nil {
				confige.Close()
				configf.Close()
			} else {
				log("WARNING: yaml.Encoder.Encode: %v", err)
				return err
			}
		} else {
			log("WARNING: os.Create config file %s: %v", YamlConfigPath, err)
			return err
		}
	} else {
		log("WARNING: os.Open config file %s: %v", YamlConfigPath, err)
		return err
	}

	return nil
}

func KvGet(name string) (value string, err error) {
	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s/values/%s", KvAccountId, KvNamespaceId, name),
		nil,
	)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", KvToken))
	resp, err := HttpClient.Do(req)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("kv api response status: %s", resp.Status)
	}

	if rbb, err := io.ReadAll(resp.Body); err != nil {
		return "", err
	} else {
		value = string(rbb)
	}

	return value, nil
}

func KvSet(name, value string) error {
	mpbb := new(bytes.Buffer)
	mpw := multipart.NewWriter(mpbb)
	if err := mpw.WriteField("metadata", "{}"); err != nil {
		return err
	}
	if err := mpw.WriteField("value", value); err != nil {
		return err
	}
	mpw.Close()

	req, err := http.NewRequest(
		"PUT",
		fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s/values/%s", KvAccountId, KvNamespaceId, name),
		mpbb,
	)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", mpw.FormDataContentType())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", KvToken))
	resp, err := HttpClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("kv api response status: %s", resp.Status)
	}

	return nil
}

func init() {
	var err error

	if os.Getenv("YamlConfigPath") != "" {
		YamlConfigPath = os.Getenv("YamlConfigPath")
	}
	if YamlConfigPath == "" {
		log("WARNING: YamlConfigPath empty")
	}

	KvToken = GetVar("KvToken")
	if KvToken == "" {
		log("WARNING: KvToken empty")
	}

	KvAccountId = GetVar("KvAccountId")
	if KvAccountId == "" {
		log("WARNING: KvAccountId empty")
	}

	KvNamespaceId = GetVar("KvNamespaceId")
	if KvNamespaceId == "" {
		log("WARNING: KvNamespaceId empty")
	}

	Ctx = context.TODO()

	TgToken = GetVar("TgToken")
	if TgToken == "" {
		log("ERROR: TgToken empty")
		os.Exit(1)
	}

	if GetVar("TgZeChatId") == "" {
		log("ERROR: TgZeChatId empty")
		os.Exit(1)
	} else {
		TgZeChatId, err = strconv.ParseInt(GetVar("TgZeChatId"), 10, 0)
		if err != nil {
			log("ERROR: invalid TgZeChatId: %v", err)
			os.Exit(1)
		}
	}

	if GetVar("MoonPhaseTgChatId") == "" {
		MoonPhaseTgChatId = TgZeChatId
	} else {
		MoonPhaseTgChatId, err = strconv.ParseInt(GetVar("MoonPhaseTgChatId"), 10, 0)
		if err != nil {
			log("ERROR: invalid MoonPhaseTgChatId: %v", err)
			os.Exit(1)
		}
	}
	MoonPhaseTodayLast = GetVar("MoonPhaseTodayLast")

	if GetVar("ABookOfDaysPath") != "" {
		ABookOfDaysPath = GetVar("ABookOfDaysPath")
		if GetVar("ABookOfDaysRe") == "" {
			log("ABookOfDaysRe env is empty")
			os.Exit(1)
		} else {
			ABookOfDaysRe = regexp.MustCompile(GetVar("ABookOfDaysRe"))
		}
		if GetVar("ABookOfDaysTgChatId") == "" {
			log("ABookOfDaysTgChatId env is empty")
			os.Exit(1)
		}
	}
	ABookOfDaysLast = GetVar("ABookOfDaysLast")
	if GetVar("ABookOfDaysTgChatId") != "" {
		ABookOfDaysTgChatId, err = strconv.ParseInt(GetVar("ABookOfDaysTgChatId"), 10, 0)
		if err != nil {
			log("ERROR: invalid ABookOfDaysTgChatId: %v", err)
			os.Exit(1)
		}
	}

	if GetVar("ACourseInMiraclesWorkbookPath") != "" {
		ACourseInMiraclesWorkbookPath = GetVar("ACourseInMiraclesWorkbookPath")
		if GetVar("ACourseInMiraclesWorkbookTgChatId") == "" {
			log("ACourseInMiraclesWorkbookTgChatId env is empty")
			os.Exit(1)
		}
	}
	ACourseInMiraclesWorkbookRe = regexp.MustCompile(ACourseInMiraclesWorkbookReString)
	ACourseInMiraclesWorkbookLast = GetVar("ACourseInMiraclesWorkbookLast")
	if GetVar("ACourseInMiraclesWorkbookTgChatId") != "" {
		ACourseInMiraclesWorkbookTgChatId, err = strconv.ParseInt(GetVar("ACourseInMiraclesWorkbookTgChatId"), 10, 0)
		if err != nil {
			log("ERROR: invalid ACourseInMiraclesWorkbookTgChatId: %v", err)
			os.Exit(1)
		}
	}
}

func PostACourseInMiraclesWorkbook() error {
	if ACourseInMiraclesWorkbookPath == "" || time.Now().UTC().Hour() < 4 {
		return nil
	}

	if time.Now().UTC().Month() == 3 && time.Now().UTC().Day() == 1 && ACourseInMiraclesWorkbookLast != "* LESSON 1 *" {
		ACourseInMiraclesWorkbookLast = ""
	}

	var ty0 time.Time
	if time.Now().UTC().Month() < 3 {
		ty0 = time.Date(time.Now().UTC().Year()-1, time.Month(3), 1, 0, 0, 0, 0, time.UTC)
	} else {
		ty0 = time.Date(time.Now().UTC().Year(), time.Month(3), 1, 0, 0, 0, 0, time.UTC)
	}
	daynum := time.Since(ty0)/(24*time.Hour) + 1
	daynums := fmt.Sprintf(" %d ", daynum)

	acimwbbb, err := ioutil.ReadFile(ACourseInMiraclesWorkbookPath)
	if err != nil {
		return fmt.Errorf("ReadFile ACourseInMiraclesWorkbookPath=`%s`: %v", ACourseInMiraclesWorkbookPath, err)
	}
	acimwb := string(acimwbbb)
	if acimwb == "" {
		return fmt.Errorf("Empty file ACourseInMiraclesWorkbookPath=`%s`", ACourseInMiraclesWorkbookPath)
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

	if strings.Contains(ACourseInMiraclesWorkbookLast, daynums) {
		return nil
	}

	var skip bool
	if ACourseInMiraclesWorkbookLast != "" {
		skip = true
	}

	for _, s := range acimwbss {
		st := strings.Split(s, "\n")[0]
		if st == ACourseInMiraclesWorkbookLast {
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
			_, err = tgsendMessage(message, ACourseInMiraclesWorkbookTgChatId, "MarkdownV2")
			if err != nil {
				return fmt.Errorf("tgsendMessage: %v", err)
			}
		}

		ACourseInMiraclesWorkbookLast = st

		err = SetVar("ACourseInMiraclesWorkbookLast", ACourseInMiraclesWorkbookLast)
		if err != nil {
			return fmt.Errorf("SetVar ACourseInMiraclesWorkbookLast: %v", err)
		}

		if ACourseInMiraclesWorkbookRe.MatchString(st) {
			break
		}
	}

	return nil
}

func PostABookOfDays() error {
	if ABookOfDaysPath == "" || time.Now().UTC().Hour() < 4 {
		return nil
	}

	abodbb, err := ioutil.ReadFile(ABookOfDaysPath)
	if err != nil {
		return fmt.Errorf("ReadFile ABookOfDaysPath=`%s`: %v", ABookOfDaysPath, err)
	}
	abod := string(abodbb)
	if abod == "" {
		return fmt.Errorf("Empty file ABookOfDaysPath=`%s`", ABookOfDaysPath)
	}

	monthday := time.Now().UTC().Format("January 2")
	if GetVar("ABookOfDaysRe") == "" {
		return fmt.Errorf("ABookOfDaysRe env is empty")
	}
	ABookOfDaysRe := regexp.MustCompile(strings.ReplaceAll(GetVar("ABookOfDaysRe"), "monthday", monthday))
	abodtoday := ABookOfDaysRe.FindString(abod)
	abodtoday = strings.TrimSpace(abodtoday)
	if abodtoday == "" {
		log("Could not find A Book of Days text for today")
		return nil
	}

	//log("abodtoday:\n%s", abodtoday)

	if monthday == ABookOfDaysLast {
		return nil
	}

	_, err = tgsendMessage(abodtoday, ABookOfDaysTgChatId, "MarkdownV2")
	if err != nil {
		return fmt.Errorf("tgsendMessage: %v", err)
	}

	err = SetVar("ABookOfDaysLast", monthday)
	if err != nil {
		return fmt.Errorf("SetVar ABookOfDaysLast: %v", err)
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

func PostMoonPhaseToday() error {
	var err error

	if time.Now().UTC().Hour() < 4 {
		return nil
	}

	moonphase := MoonPhaseToday()

	yearmonthday := time.Now().UTC().Format("2006 January 2")
	if yearmonthday == MoonPhaseTodayLast {
		return nil
	}

	if moonphase != "" {
		_, err = tgsendMessage(moonphase, MoonPhaseTgChatId, "MarkdownV2")
		if err != nil {
			return fmt.Errorf("tgsendMessage: %v", err)
		}
	}

	err = SetVar("MoonPhaseTodayLast", yearmonthday)
	if err != nil {
		return fmt.Errorf("SetVar MoonPhaseTodayLast: %v", err)
	}

	return nil
}

func tgsendMessage(text string, chatid int64, parsemode string) (msg *TgMessage, err error) {
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
	sendMessage := map[string]interface{}{
		"chat_id":                  chatid,
		"text":                     text,
		"parse_mode":               parsemode,
		"disable_web_page_preview": true,
	}
	sendMessageJSON, err := json.Marshal(sendMessage)
	if err != nil {
		return nil, err
	}

	var tgresp TgResponse
	err = postJson(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", TgToken),
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

func main() {
	var err error

	err = PostMoonPhaseToday()
	if err != nil {
		log("WARNING: PostMoonPhaseToday: %v", err)
	}

	err = PostABookOfDays()
	if err != nil {
		log("WARNING: PostABookOfDays: %v", err)
	}

	err = PostACourseInMiraclesWorkbook()
	if err != nil {
		log("WARNING: PostACourseInMiraclesWorkbook: %v", err)
	}

}
