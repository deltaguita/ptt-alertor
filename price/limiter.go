package price

import (
	"os"
	"strconv"
	"sync"
	"time"
)

// defaultRPM sits below the free tier's allowance -- measured at 15 requests a
// minute for gemini-3.5-flash-lite -- leaving room for the clock skew between
// this process and the quota's own window.
const defaultRPM = 12

// limiter paces calls to the API. Exceeding the allowance fails a request
// outright rather than queueing it, and a failed extraction is a listing that
// goes unrecorded, so the pacing has to happen before the call rather than as a
// reaction to being refused.
//
// One limiter is shared by every caller: a survey walking history and an alert
// pricing a new listing draw on the same quota, and either alone staying under
// the limit says nothing about the two together.
type limiter struct {
	mu   sync.Mutex
	gap  time.Duration
	next time.Time
}

var apiLimiter = newLimiter(envRPM())

func envRPM() int {
	rpm, err := strconv.Atoi(os.Getenv("GEMINI_RPM"))
	if err != nil || rpm <= 0 {
		return defaultRPM
	}
	return rpm
}

func newLimiter(rpm int) *limiter {
	return &limiter{gap: time.Minute / time.Duration(rpm)}
}

// wait blocks until the caller's turn. Turns are claimed under the lock and
// waited for outside it, so callers queue in order instead of all waking to
// race for the same slot.
func (l *limiter) wait() {
	l.mu.Lock()
	now := time.Now()
	if l.next.Before(now) {
		l.next = now
	}
	turn := l.next
	l.next = l.next.Add(l.gap)
	l.mu.Unlock()

	if delay := time.Until(turn); delay > 0 {
		time.Sleep(delay)
	}
}
