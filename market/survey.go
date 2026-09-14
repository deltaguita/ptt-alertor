package market

import (
	"os"
	"regexp"
	"strconv"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/price"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/web"
	"github.com/garyburd/redigo/redis"
)

// surveyTitle selects the listings worth reading. Filtering on the listing page,
// before anything is downloaded, is what keeps the survey affordable: iPhone
// listings are about a fifth of the board, so four in five articles cost nothing
// beyond the index page that was fetched anyway.
var surveyTitle = regexp.MustCompile(`(?i)i?phone\s*1[2-9]`)

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
	Board    string
	Budget   int
	Pause    time.Duration
	Boundary time.Duration
}

// NewSurvey builds a survey from the environment.
func NewSurvey(board string) *Survey {
	return &Survey{
		Board:    board,
		Budget:   envInt("MARKET_SURVEY_BUDGET", defaultBudget),
		Pause:    time.Duration(envInt("MARKET_SURVEY_PAUSE_MS", int(defaultPause/time.Millisecond))) * time.Millisecond,
		Boundary: time.Duration(envInt("MARKET_SURVEY_DAYS", int(defaultBoundary.Hours()/24))) * 24 * time.Hour,
	}
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

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
	// The two newest pages are where anything new appears.
	for page := newest; page > newest-2 && budget > 0; page-- {
		budget -= s.scanPage(page, seen, budget)
	}
	if budget <= 0 {
		return
	}

	next := s.resumePage(newest)
	for ; next > 0 && budget > 0; next-- {
		if s.pageIsOlderThanBoundary(next) {
			log.WithField("board", s.Board).Info("Market Survey Reached Boundary")
			s.saveResumePage(0)
			return
		}
		budget -= s.scanPage(next, seen, budget)
	}
	s.saveResumePage(next)
}

// scanPage records what one listing page offers, returning how much budget it
// used.
func (s *Survey) scanPage(page int, seen map[string]bool, budget int) int {
	time.Sleep(s.Pause)
	articles, err := web.FetchArticles(s.Board, page)
	if err != nil {
		log.WithFields(log.Fields{"board": s.Board, "page": page}).WithError(err).
			Warn("Market Survey Page Failed")
		return 0
	}
	spent := 0
	records := make([]Record, 0)
	for _, listed := range articles {
		if spent >= budget {
			break
		}
		if !surveyPrefix.MatchString(listed.Title) || !surveyTitle.MatchString(listed.Title) {
			continue
		}
		code := listed.ParseCode()
		if code == "" || seen[code] {
			continue
		}
		spent++
		seen[code] = true
		time.Sleep(s.Pause)
		records = append(records, s.read(listed, code)...)
	}
	if err := Append(records); err != nil {
		log.WithError(err).Error("Market Survey Could Not Save Records")
	}
	return spent
}

func (s *Survey) read(listed article.Article, code string) []Record {
	info, err := price.Of(s.Board, code, func() (string, error) {
		fetched, err := web.FetchArticle(s.Board, code)
		return fetched.Content, err
	})
	if err != nil {
		log.WithFields(log.Fields{"board": s.Board, "code": code}).WithError(err).
			Warn("Market Survey Extraction Failed")
		return nil
	}
	posted := postedAt(code)
	records := make([]Record, 0, len(info.Items))
	for _, item := range info.Items {
		// Only phones carry a model, and a record without one cannot be
		// compared against anything.
		//
		// Capacity is required as well, and it is what keeps accessories out:
		// the board's rules make every genuine phone listing state its capacity,
		// while "iPhone 16 原廠矽膠保護殼" has none -- and at 699 it would drag
		// an iPhone 16 distribution down by an order of magnitude.
		if item.Model == "" || item.CapacityGB == 0 || item.Price <= 0 {
			continue
		}
		records = append(records, Record{
			Board: s.Board, Code: code, Author: listed.Author, Title: listed.Title,
			PostedAt: posted, ObservedAt: time.Now(),
			Model: item.Model, Variant: item.Variant, CapacityGB: item.CapacityGB,
			BatteryHealth: item.BatteryHealth, Price: item.Price,
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

func (s *Survey) pageIsOlderThanBoundary(page int) bool {
	articles, err := web.FetchArticles(s.Board, page)
	if err != nil || len(articles) == 0 {
		return false
	}
	newestOnPage := time.Time{}
	for _, listed := range articles {
		if posted := postedAt(listed.ParseCode()); posted.After(newestOnPage) {
			newestOnPage = posted
		}
	}
	return newestOnPage.Before(time.Now().Add(-s.Boundary))
}

func (s *Survey) resumeKey() string { return "market:survey:" + s.Board + ":page" }

// resumePage reports where the last run stopped walking backwards, defaulting to
// just behind the newest pages on a first run.
func (s *Survey) resumePage(newest int) int {
	conn := connections.Redis()
	defer conn.Close()
	page, err := redis.Int(conn.Do("GET", s.resumeKey()))
	if err != nil || page <= 0 {
		return newest - 2
	}
	return page
}

func (s *Survey) saveResumePage(page int) {
	conn := connections.Redis()
	defer conn.Close()
	if _, err := conn.Do("SET", s.resumeKey(), page); err != nil {
		log.WithError(err).Error("Market Survey Could Not Save Progress")
	}
}
