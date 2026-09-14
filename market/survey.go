package market

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/price"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/web"
	"github.com/garyburd/redigo/redis"
)

// surveyPrefix keeps announcements and reviews out; only listings and want-ads
// carry a price.
var surveyPrefix = regexp.MustCompile(`^\[(販售|徵求)\]`)

const (
	defaultBudget   = 200
	defaultPause    = 2 * time.Second
	defaultBoundary = 180 * 24 * time.Hour
)

// Survey walks a board and records the asking prices it finds.
//
// Each run works the newest pages first so the recent picture is always current,
// then spends whatever budget is left walking backwards through history. It
// stops when the budget runs out and resumes from the same page next time, so a
// six-month backfill costs a few runs rather than one long burst that would
// exhaust the day's API quota and draw attention from the board.
type Survey struct {
	// Kinds are the classes of goods this survey reads. One pass over the board
	// serves all of them: the listing pages are fetched once regardless, and an
	// article is only downloaded when some kind's title pattern wants it.
	Kinds    []Kind
	Board    string
	Budget   int
	Pause    time.Duration
	Boundary time.Duration
}

// NewSurvey builds a survey over every registered kind.
func NewSurvey(board string) *Survey {
	return &Survey{
		Kinds:    Kinds(),
		Board:    board,
		Budget:   envInt("MARKET_SURVEY_BUDGET", defaultBudget),
		Pause:    time.Duration(envInt("MARKET_SURVEY_PAUSE_MS", int(defaultPause/time.Millisecond))) * time.Millisecond,
		Boundary: time.Duration(envInt("MARKET_SURVEY_DAYS", int(defaultBoundary.Hours()/24))) * 24 * time.Hour,
	}
}

// Boards reports the boards to survey, from MARKET_SURVEY_BOARDS.
func Boards() []string {
	configured := strings.Split(os.Getenv("MARKET_SURVEY_BOARDS"), ",")
	boards := make([]string, 0, len(configured))
	for _, board := range configured {
		if board = strings.TrimSpace(board); board != "" {
			boards = append(boards, board)
		}
	}
	if len(boards) == 0 {
		return []string{"macshop"}
	}
	return boards
}

// ResetBackfill makes the next run walk history again from the newest pages.
// Widening the pattern or the boundary only affects listings the survey has yet
// to see, so this is what gives a newly tracked product its past.
func (s *Survey) ResetBackfill() {
	conn := connections.Redis()
	defer conn.Close()
	if _, err := conn.Do("DEL", s.resumeKey()); err != nil {
		log.WithError(err).Error("Market Survey Could Not Reset Progress")
	}
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

// backfillDone stands in for a page number once the walk has reached the
// boundary. It has to be distinguishable from an absent key: treating "finished"
// as "never started" sends every later run back over the whole history, which
// costs nothing in extractions -- every article is already recorded -- but
// re-fetches hundreds of listing pages to discover that.
const backfillDone = -1

// Run implements cron.Job.
func (s *Survey) Run() {
	newest, err := web.CurrentPage(s.Board)
	if err != nil {
		log.WithField("board", s.Board).WithError(err).Error("Market Survey Could Not Read Board")
		return
	}
	seen, err := Codes(time.Now().Add(-s.Boundary))
	if err != nil {
		log.WithError(err).Error("Market Survey Could Not Read History")
		return
	}

	budget := s.Budget
	// The two newest pages are where anything new appears, so they are worked
	// first on every run: the recent picture stays current even while a backfill
	// is still grinding through history.
	for page := newest; page > newest-2 && budget > 0; page-- {
		spent, _, _ := s.scanPage(page, seen, budget)
		budget -= spent
	}

	next, done := s.resumePage(newest)
	if done || budget <= 0 {
		return
	}
	for ; next > 0 && budget > 0; next-- {
		spent, newestOnPage, ok := s.scanPage(next, seen, budget)
		budget -= spent
		// The boundary is judged from the page just read rather than by fetching
		// it again.
		if ok && newestOnPage.Before(time.Now().Add(-s.Boundary)) {
			log.WithFields(log.Fields{"board": s.Board, "page": next}).
				Info("Market Survey Reached Boundary")
			s.saveResumePage(backfillDone)
			return
		}
	}
	s.saveResumePage(next)
}

// scanPage records what one listing page offers. It returns how much budget it
// used, when the newest article on the page was posted, and whether the page
// could be read at all.
func (s *Survey) scanPage(page int, seen map[string]bool, budget int) (int, time.Time, bool) {
	time.Sleep(s.Pause)
	articles, err := web.FetchArticles(s.Board, page)
	if err != nil {
		log.WithFields(log.Fields{"board": s.Board, "page": page}).WithError(err).
			Warn("Market Survey Page Failed")
		return 0, time.Time{}, false
	}
	spent := 0
	newestOnPage := time.Time{}
	records := make([]Record, 0)
	for _, listed := range articles {
		code := listed.ParseCode()
		if code == "" {
			continue
		}
		if posted := postedAt(code); posted.After(newestOnPage) {
			newestOnPage = posted
		}
		if spent >= budget {
			continue
		}
		if !surveyPrefix.MatchString(listed.Title) || seen[code] {
			continue
		}
		wanted := s.kindsFor(listed.Title)
		if len(wanted) == 0 {
			continue
		}
		spent++
		seen[code] = true
		for _, kind := range wanted {
			time.Sleep(s.Pause)
			records = append(records, s.read(kind, listed, code)...)
		}
	}
	if err := Append(records); err != nil {
		log.WithError(err).Error("Market Survey Could Not Save Records")
	}
	return spent, newestOnPage, true
}

// kindsFor returns the kinds whose title pattern claims a listing.
func (s *Survey) kindsFor(title string) []Kind {
	wanted := make([]Kind, 0, 1)
	for _, kind := range s.Kinds {
		if kind.TitlePattern().MatchString(title) {
			wanted = append(wanted, kind)
		}
	}
	return wanted
}

func (s *Survey) read(kind Kind, listed article.Article, code string) []Record {
	info, err := price.Of(kind, s.Board, code, func() (string, error) {
		fetched, err := web.FetchArticle(s.Board, code)
		return fetched.Content, err
	})
	if err != nil {
		log.WithFields(log.Fields{"board": s.Board, "code": code, "kind": kind.Name()}).
			WithError(err).Warn("Market Survey Extraction Failed")
		return nil
	}
	posted := postedAt(code)
	records := make([]Record, 0, len(info.Items))
	for _, item := range info.Items {
		if item.Price <= 0 {
			continue
		}
		// The kind decides whether an item is one of its own: a phone case named
		// "iPhone 16 保護殼" is not an iPhone, and recording it at 699 would drag
		// the iPhone 16 distribution down by an order of magnitude.
		attrs, ok := kind.Attrs(item)
		if !ok {
			continue
		}
		records = append(records, Record{
			Kind: kind.Name(), Board: s.Board, Code: code,
			Author: listed.Author, Title: listed.Title,
			PostedAt: posted, ObservedAt: time.Now(),
			Attrs: attrs, Price: item.Price,
			Sold: info.IsSold, PostType: info.PostType,
		})
	}
	return records
}

var codeTime = regexp.MustCompile(`^[GM]\.(\d+)\.`)

func postedAt(code string) time.Time {
	matches := codeTime.FindStringSubmatch(code)
	if len(matches) < 2 {
		return time.Now()
	}
	seconds, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return time.Now()
	}
	return time.Unix(seconds, 0)
}

func (s *Survey) resumeKey() string { return "market:survey:" + s.Board + ":page" }

// resumePage reports where the last run stopped walking backwards and whether
// the walk has already reached the boundary. A first run starts just behind the
// newest pages.
func (s *Survey) resumePage(newest int) (int, bool) {
	conn := connections.Redis()
	defer conn.Close()
	page, err := redis.Int(conn.Do("GET", s.resumeKey()))
	return resumeFrom(page, err == nil, newest)
}

// resumeFrom interprets stored progress. Finished and never-started have to lead
// to different places: conflating them is what made a completed survey walk the
// whole board again on every run.
func resumeFrom(stored int, found bool, newest int) (int, bool) {
	switch {
	case !found:
		return newest - 2, false
	case stored == backfillDone:
		return 0, true
	case stored <= 0:
		return newest - 2, false
	default:
		return stored, false
	}
}

func (s *Survey) saveResumePage(page int) {
	conn := connections.Redis()
	defer conn.Close()
	if _, err := conn.Do("SET", s.resumeKey(), page); err != nil {
		log.WithError(err).Error("Market Survey Could Not Save Progress")
	}
}
