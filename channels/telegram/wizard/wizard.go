// Package wizard holds the state of a step-by-step subscription, kept apart from
// the Telegram plumbing so that the part with the rules in it can be tested
// without a bot to talk to.
package wizard

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/garyburd/redigo/redis"
)

// A subscription needs four things -- a board, a keyword, what to exclude and a
// price ceiling -- and only the keyword cannot be offered as a button. The
// wizard asks for them one at a time so that nobody has to remember that the
// whole thing is spelled
//
//	新增售價 macshop iPhone 17 Pro&!Max 35000
//
// Its state lives in Redis rather than in memory because the bot is restarted
// whenever it is deployed, and a half-finished answer should not survive as a
// silent trap either -- hence the expiry.
const wizardTTL = 15 * time.Minute

// Steps of the wizard. Each question that can also be answered in words has a
// separate typing step, so an arriving message is never mistaken for an answer
// to a question that only takes buttons.
const (
	StepBoard         = "board"
	StepBoardTyping   = "board?"
	StepKeyword       = "keyword"
	StepExclude       = "exclude"
	StepExcludeTyping = "exclude?"
	StepPrice         = "price"
	StepPriceTyping   = "price?"
	StepConfirm       = "confirm"
)

// AwaitsText reports whether the wizard is waiting for something to be typed.
func (w *Wizard) AwaitsText() bool {
	switch w.Step {
	case StepBoardTyping, StepKeyword, StepExcludeTyping, StepPriceTyping:
		return true
	default:
		return false
	}
}

type Wizard struct {
	Step    string `json:"step"`
	Board   string `json:"board"`
	Keyword string `json:"keyword"`
	Exclude string `json:"exclude"`
	// Price is the ceiling; zero means the subscription is on the title alone.
	Price  int   `json:"price"`
	ChatID int64 `json:"chatID"`
}

func wizardKey(userID string) string { return "tg:wizard:" + userID }

func Load(userID string) (*Wizard, bool) {
	conn := connections.Redis()
	defer conn.Close()
	stored, err := redis.Bytes(conn.Do("GET", wizardKey(userID)))
	if err != nil {
		if err != redis.ErrNil {
			log.WithError(err).Error("Telegram Wizard Read Failed")
		}
		return nil, false
	}
	var w Wizard
	if err := json.Unmarshal(stored, &w); err != nil {
		log.WithError(err).Warn("Telegram Wizard Decode Failed")
		return nil, false
	}
	return &w, true
}

func (w *Wizard) Save(userID string) {
	encoded, err := json.Marshal(w)
	if err != nil {
		log.WithError(err).Error("Telegram Wizard Encode Failed")
		return
	}
	conn := connections.Redis()
	defer conn.Close()
	if _, err := conn.Do("SET", wizardKey(userID), encoded, "EX", int(wizardTTL.Seconds())); err != nil {
		log.WithError(err).Error("Telegram Wizard Write Failed")
	}
}

func Clear(userID string) {
	conn := connections.Redis()
	defer conn.Close()
	if _, err := conn.Do("DEL", wizardKey(userID)); err != nil {
		log.WithError(err).Error("Telegram Wizard Clear Failed")
	}
}

// Command renders the wizard's answers as the command a subscriber would
// otherwise have had to type. Building the real command rather than calling the
// actions directly keeps one code path for subscriptions, so the wizard cannot
// drift away from what the typed form does.
func (w *Wizard) Command() string {
	keyword := w.Keyword
	if w.Exclude != "" {
		keyword += "&!" + w.Exclude
	}
	if w.Price > 0 {
		return "新增售價 " + w.Board + " " + keyword + " " + strconv.Itoa(w.Price)
	}
	return "新增 " + w.Board + " " + keyword
}

// Summary describes what is about to be subscribed, in words rather than syntax.
func (w *Wizard) Summary() string {
	lines := []string{
		"看板：" + w.Board,
		"關鍵字：" + w.Keyword,
	}
	if w.Exclude != "" {
		lines = append(lines, "排除：標題含「"+w.Exclude+"」的不通知")
	}
	if w.Price > 0 {
		lines = append(lines, "售價上限："+strconv.Itoa(w.Price))
	} else {
		lines = append(lines, "售價上限：不限（只看標題）")
	}
	return strings.Join(lines, "\n")
}

// SuggestsProMax reports whether a keyword would also catch the Max variant, in
// which case offering to exclude it is worth a button. Pro and Pro Max share a
// prefix, so a subscription to one silently includes the other.
func SuggestsProMax(keyword string) bool {
	lower := strings.ToLower(keyword)
	return strings.Contains(lower, "pro") && !strings.Contains(lower, "max")
}
