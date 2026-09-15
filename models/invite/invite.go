// Package invite issues and redeems the codes that let someone use the bot.
//
// The bot runs on one small machine against a shared API allowance, so who may
// subscribe has to be decided rather than left open. Codes are used instead of
// one shared passphrase because a passphrase cannot be taken back from one
// person: it spreads, and the only remedy is to change it for everybody.
package invite

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"sort"
	"strings"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/garyburd/redigo/redis"
)

// alphabet leaves out the characters that are read wrongly when a code is
// copied by hand or over the phone: 0/O, 1/I/l.
const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

const codeLength = 8

const prefix = "invite:"

// Code is one invitation.
type Code struct {
	Code       string    `json:"code"`
	CreatedAt  time.Time `json:"createdAt"`
	CreatedBy  string    `json:"createdBy"`
	Note       string    `json:"note,omitempty"`
	RedeemedBy string    `json:"redeemedBy,omitempty"`
	RedeemedAt time.Time `json:"redeemedAt,omitempty"`
}

// Redeemed reports whether the code has been used.
func (c Code) Redeemed() bool { return c.RedeemedBy != "" }

// Looks reports whether a piece of text has the shape of a code, so that an
// ordinary message is not treated as a failed redemption.
func Looks(text string) bool {
	text = strings.ToUpper(strings.TrimSpace(text))
	if len(text) != codeLength {
		return false
	}
	for _, r := range text {
		if !strings.ContainsRune(alphabet, r) {
			return false
		}
	}
	return true
}

// New issues a code. The note is for the issuer's own records -- who it was
// meant for -- so that a code can be matched to a person later.
func New(createdBy, note string) (Code, error) {
	code := Code{
		Code:      generate(),
		CreatedAt: time.Now(),
		CreatedBy: createdBy,
		Note:      note,
	}
	return code, save(code)
}

func generate() string {
	letters := make([]byte, codeLength)
	for i := range letters {
		// crypto/rand, not math/rand: a guessable code is no gate at all.
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			log.WithError(err).Error("Invite Code Generation Failed")
			return ""
		}
		letters[i] = alphabet[n.Int64()]
	}
	return string(letters)
}

func key(code string) string { return prefix + strings.ToUpper(code) }

func save(code Code) error {
	encoded, err := json.Marshal(code)
	if err != nil {
		return err
	}
	conn := connections.Redis()
	defer conn.Close()
	_, err = conn.Do("SET", key(code.Code), encoded)
	return err
}

// Find returns a code, reporting false when there is no such code.
func Find(code string) (Code, bool) {
	conn := connections.Redis()
	defer conn.Close()
	stored, err := redis.Bytes(conn.Do("GET", key(code)))
	if err != nil {
		if err != redis.ErrNil {
			log.WithError(err).Error("Invite Read Failed")
		}
		return Code{}, false
	}
	var found Code
	if err := json.Unmarshal(stored, &found); err != nil {
		log.WithError(err).Warn("Invite Decode Failed")
		return Code{}, false
	}
	return found, true
}

// ErrUsed and ErrUnknown say why a redemption failed, so the caller can tell a
// mistyped code from one that has already been spent.
var (
	ErrUnknown = redeemError("查無此邀請碼")
	ErrUsed    = redeemError("這張邀請碼已經被使用過了")
)

type redeemError string

func (e redeemError) Error() string { return string(e) }

// Redeem marks a code as used by an account.
func Redeem(code, account string) error {
	found, ok := Find(code)
	if !ok {
		return ErrUnknown
	}
	if found.Redeemed() {
		return ErrUsed
	}
	found.RedeemedBy = account
	found.RedeemedAt = time.Now()
	return save(found)
}

// All returns every code, newest first.
func All() []Code {
	conn := connections.Redis()
	defer conn.Close()
	keys, err := redis.Strings(conn.Do("KEYS", prefix+"*"))
	if err != nil {
		log.WithError(err).Error("Invite List Failed")
		return nil
	}
	codes := make([]Code, 0, len(keys))
	for _, k := range keys {
		if code, ok := Find(strings.TrimPrefix(k, prefix)); ok {
			codes = append(codes, code)
		}
	}
	sort.Slice(codes, func(i, j int) bool { return codes[i].CreatedAt.After(codes[j].CreatedAt) })
	return codes
}

// Revoke deletes an unredeemed code.
func Revoke(code string) bool {
	conn := connections.Redis()
	defer conn.Close()
	removed, err := redis.Int(conn.Do("DEL", key(code)))
	if err != nil {
		log.WithError(err).Error("Invite Revoke Failed")
		return false
	}
	return removed > 0
}
