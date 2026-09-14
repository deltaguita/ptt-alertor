// Package price extracts asking prices from PTT second-hand board articles.
//
// Board articles follow a template that the board rules enforce, but the
// template is only loosely observed: prices appear as "[售價]", "［售價］" or
// "價格", multi-item posts number their prices, and the body is full of numbers
// that are not prices at all (capacities, battery health, warranty dates, model
// numbers, shipping fees, and the board's own boilerplate). Extraction is
// therefore delegated to an LLM, which also reports whether the item is already
// sold and whether the post is a want-ad -- neither of which a pattern match can
// determine.
package price

import (
	"encoding/json"
	"sync"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/garyburd/redigo/redis"
)

// cacheTTL bounds the cache. Board articles are irrelevant long before this,
// and the bound is what keeps the key set from growing without limit -- the
// Redis instance runs with noeviction, so nothing else would reclaim them.
const cacheTTL = 30 * 24 * time.Hour

const (
	PostTypeSale   = "販售"
	PostTypeWanted = "徵求"
)

const (
	ConfidenceHigh = "high"
	ConfidenceLow  = "low"
)

// Item is a single item offered in an article. A post that sells several things
// yields several items; one that sells a bundle "不拆賣" yields a single item
// priced at the bundle price.
type Item struct {
	Name  string `json:"name"`
	Price int    `json:"price"`
}

// Info is what the extractor reports about an article.
type Info struct {
	PostType   string `json:"post_type"`
	IsSold     bool   `json:"is_sold"`
	Items      []Item `json:"items"`
	Confidence string `json:"confidence"`
}

// Extractor reports the trade information described by an article body.
type Extractor interface {
	Extract(content string) (Info, error)
}

// extractor is swapped out in tests.
var extractor Extractor = NewGemini()

// Decision is what the caller should do with an article.
type Decision int

const (
	// Skip means the article does not warrant a notification: it is already
	// sold, it is a want-ad, or every item costs more than the subscriber asked.
	Skip Decision = iota
	// Notify means an item matched and its price is known.
	Notify
	// NotifyUnverified means the price could not be established. The article is
	// still worth sending: a missed bargain costs the subscriber more than a
	// notification they did not need.
	NotifyUnverified
)

// Decide reports what to do with an article given a subscriber's price ceiling,
// along with the matching price when there is one.
func Decide(info Info, maxPrice int) (Decision, int) {
	if info.IsSold || info.PostType == PostTypeWanted {
		return Skip, 0
	}
	if info.Confidence != ConfidenceHigh || len(info.Items) == 0 {
		return NotifyUnverified, 0
	}
	best, found := 0, false
	for _, item := range info.Items {
		if item.Price <= 0 || item.Price > maxPrice {
			continue
		}
		if !found || item.Price < best {
			best, found = item.Price, true
		}
	}
	if !found {
		return Skip, 0
	}
	return Notify, best
}

// flight is one in-progress extraction, shared by everyone who asks for the
// same article while it runs.
type flight struct {
	wg   sync.WaitGroup
	info Info
	err  error
}

var (
	flightsMu sync.Mutex
	flights   = make(map[string]*flight)
)

// Of returns the trade information for an article, fetching and extracting it
// only the first time it is asked for. Article bodies never change once posted,
// so a cached answer stays correct for as long as it is kept.
//
// fetchContent is called only on a cache miss, and only once even when several
// subscribers' keywords match the same article at the same moment: a board check
// runs one goroutine per keyword per subscriber, so without this the same
// article would be downloaded and extracted once per match.
func Of(board, code string, fetchContent func() (string, error)) (Info, error) {
	if info, ok := lookup(board, code); ok {
		return info, nil
	}

	key := cacheKey(board, code)
	flightsMu.Lock()
	if inFlight, ok := flights[key]; ok {
		flightsMu.Unlock()
		inFlight.wg.Wait()
		return inFlight.info, inFlight.err
	}
	current := new(flight)
	current.wg.Add(1)
	flights[key] = current
	flightsMu.Unlock()

	current.info, current.err = fetchAndExtract(board, code, fetchContent)

	flightsMu.Lock()
	delete(flights, key)
	flightsMu.Unlock()
	current.wg.Done()

	return current.info, current.err
}

func fetchAndExtract(board, code string, fetchContent func() (string, error)) (Info, error) {
	// Re-check: a caller that waited on an earlier flight for this article may
	// have already stored the answer.
	if info, ok := lookup(board, code); ok {
		return info, nil
	}
	content, err := fetchContent()
	if err != nil {
		return Info{}, err
	}
	info, err := extractor.Extract(content)
	if err != nil {
		return Info{}, err
	}
	store(board, code, info)
	return info, nil
}

func cacheKey(board, code string) string {
	return "price:" + board + ":" + code
}

func lookup(board, code string) (Info, bool) {
	conn := connections.Redis()
	defer conn.Close()
	cached, err := redis.Bytes(conn.Do("GET", cacheKey(board, code)))
	if err != nil {
		if err != redis.ErrNil {
			log.WithError(err).Error("Price Cache Read Failed")
		}
		return Info{}, false
	}
	var info Info
	if err := json.Unmarshal(cached, &info); err != nil {
		log.WithError(err).Warn("Price Cache Decode Failed")
		return Info{}, false
	}
	return info, true
}

func store(board, code string, info Info) {
	encoded, err := json.Marshal(info)
	if err != nil {
		log.WithError(err).Error("Price Cache Encode Failed")
		return
	}
	conn := connections.Redis()
	defer conn.Close()
	if _, err := conn.Do("SET", cacheKey(board, code), encoded, "EX", int(cacheTTL.Seconds())); err != nil {
		log.WithError(err).Error("Price Cache Write Failed")
	}
}
