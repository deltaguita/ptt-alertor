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
	"regexp"
	"strconv"
	"sync"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/garyburd/redigo/redis"
)

// Sellers edit their posts: a price cut a few days after posting is routine, and
// the edit leaves the article code unchanged. A cached extraction is therefore
// only as trustworthy as the article is settled, so an article still inside its
// editing window is cached just long enough to spare a burst of duplicate calls,
// while one that has settled is cached for the full retention window.
//
// settledTTL is also what keeps the key set from growing without limit -- the
// Redis instance runs with noeviction, so nothing else would reclaim these keys.
const (
	freshTTL     = 6 * time.Hour
	settledTTL   = 30 * 24 * time.Hour
	settlePeriod = 7 * 24 * time.Hour
)

// articleCodeTime matches the epoch seconds PTT embeds in an article code, which
// is when the article was posted.
var articleCodeTime = regexp.MustCompile(`^[GM]\.(\d+)\.`)

func ttlFor(code string) time.Duration {
	matches := articleCodeTime.FindStringSubmatch(code)
	if len(matches) < 2 {
		return freshTTL
	}
	posted, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return freshTTL
	}
	if time.Since(time.Unix(posted, 0)) < settlePeriod {
		return freshTTL
	}
	return settledTTL
}

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
	// Attrs holds whatever the caller's Kind asked the extractor to fill in --
	// generation and capacity for a phone, chip and memory for a laptop. They
	// are carried as strings so that this package needs to know nothing about
	// any particular class of goods; the Kind that asked for them is what
	// validates and normalises them.
	Attrs map[string]string `json:"attrs,omitempty"`
}

// Info is what the extractor reports about an article.
type Info struct {
	PostType   string `json:"post_type"`
	IsSold     bool   `json:"is_sold"`
	Items      []Item `json:"items"`
	Confidence string `json:"confidence"`
}

// Kind is what a caller must supply to have an article read: the attributes it
// wants filled in, and the rules that tell the extractor how to fill them.
type Kind interface {
	// Name identifies the kind in cache keys, so that widening what a kind asks
	// for cannot be served a reply made under the old rules.
	Name() string
	// AttrSchema returns the JSON-schema properties for this kind's attributes.
	AttrSchema() map[string]interface{}
	// PromptRules are the sentences appended to the shared extraction rules.
	PromptRules() string
}

// Extractor reports the trade information described by an article body.
type Extractor interface {
	Extract(content string, kind Kind) (Info, error)
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
// along with the cheapest item that met it. The item, not just its price, is
// what the caller needs: comparing against the market requires knowing which
// model and capacity was matched.
func Decide(info Info, maxPrice int) (Decision, Item) {
	if info.IsSold || info.PostType == PostTypeWanted {
		return Skip, Item{}
	}
	if info.Confidence != ConfidenceHigh || len(info.Items) == 0 {
		return NotifyUnverified, Item{}
	}
	best, found := Item{}, false
	for _, item := range info.Items {
		if item.Price <= 0 || item.Price > maxPrice {
			continue
		}
		if !found || item.Price < best.Price {
			best, found = item, true
		}
	}
	if !found {
		return Skip, Item{}
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
func Of(kind Kind, board, code string, fetchContent func() (string, error)) (Info, error) {
	if info, ok := lookup(kind, board, code); ok {
		return info, nil
	}

	key := cacheKey(kind, board, code)
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

	current.info, current.err = fetchAndExtract(kind, board, code, fetchContent)

	flightsMu.Lock()
	delete(flights, key)
	flightsMu.Unlock()
	current.wg.Done()

	return current.info, current.err
}

func fetchAndExtract(kind Kind, board, code string, fetchContent func() (string, error)) (Info, error) {
	// Re-check: a caller that waited on an earlier flight for this article may
	// have already stored the answer.
	if info, ok := lookup(kind, board, code); ok {
		return info, nil
	}
	content, err := fetchContent()
	if err != nil {
		return Info{}, err
	}
	info, err := extractor.Extract(content, kind)
	if err != nil {
		return Info{}, err
	}
	store(kind, board, code, info)
	return info, nil
}

// cacheKey carries a version and the kind that produced the entry. The version
// covers both the stored shape and the rules the extraction was made under --
// an entry written before accessories were excluded still claims a phone case is
// an iPhone 16, and a stale hit like that is silently wrong rather than merely
// missing. The kind keeps two kinds reading the same board from serving each
// other answers shaped for different attributes.
func cacheKey(kind Kind, board, code string) string {
	return "price:v4:" + kind.Name() + ":" + board + ":" + code
}

func lookup(kind Kind, board, code string) (Info, bool) {
	conn := connections.Redis()
	defer conn.Close()
	cached, err := redis.Bytes(conn.Do("GET", cacheKey(kind, board, code)))
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

func store(kind Kind, board, code string, info Info) {
	encoded, err := json.Marshal(info)
	if err != nil {
		log.WithError(err).Error("Price Cache Encode Failed")
		return
	}
	conn := connections.Redis()
	defer conn.Close()
	if _, err := conn.Do("SET", cacheKey(kind, board, code), encoded, "EX", int(ttlFor(code).Seconds())); err != nil {
		log.WithError(err).Error("Price Cache Write Failed")
	}
}
