package market

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/myutil"
)

// Distribution summarises what one product has been asking over a window.
type Distribution struct {
	Kind   Kind
	Query  Attrs
	Window time.Duration
	Count  int
	Sold   int
	Min    int
	Max    int
	P25    int
	Median int
	P75    int
	// Spread lists, for each attribute the query left open, the values the
	// records actually span. A query that did not pin capacity can then say what
	// it is averaging over: a 512G at 47,500 sits in the same tail as an
	// overpriced 256G, and the reader cannot tell which unless told.
	Spread  map[string][]string
	Buckets []Bucket
}

// Bucket is one price band of a distribution.
type Bucket struct {
	From  int
	To    int
	Count int
	Share float64
}

// Dedupe collapses repeat postings of the same item by the same seller, keeping
// the most recent. Sellers relist after the board's ten-day reposting limit, and
// counting each relist separately would weight those sellers more heavily than
// the rest.
//
// Records whose attributes are too coarse to identify one unit are left alone;
// merging them would discard genuinely distinct listings.
func Dedupe(kind Kind, records []Record) []Record {
	kept := make([]Record, 0, len(records))
	at := make(map[string]int, len(records))
	for _, record := range records {
		identity, ok := kind.DedupeKey(record.Attrs)
		if !ok {
			kept = append(kept, record)
			continue
		}
		key := record.Author + "|" + identity
		seen, found := at[key]
		if !found {
			at[key] = len(kept)
			kept = append(kept, record)
			continue
		}
		if record.PostedAt.After(kept[seen].PostedAt) {
			kept[seen] = record
		}
	}
	return kept
}

// Describe summarises the asking prices matching a query. Want-ads are excluded
// -- those are budgets, not asking prices -- while sold listings are kept and
// counted separately.
func Describe(kind Kind, records []Record, query Attrs, window time.Duration) Distribution {
	cutoff := time.Now().Add(-window)
	dist := Distribution{Kind: kind, Query: query, Window: window, Spread: map[string][]string{}}
	prices := make([]int, 0)
	spread := make(map[string]map[string]bool)
	for _, record := range Dedupe(kind, records) {
		if record.Kind != kind.Name() || record.PostType == "徵求" || record.Price <= 0 {
			continue
		}
		if record.PostedAt.Before(cutoff) || !record.Attrs.Matches(query) {
			continue
		}
		prices = append(prices, record.Price)
		if record.Sold {
			dist.Sold++
		}
		for _, name := range kind.GroupBy() {
			if query.Get(name) != "" || record.Attrs.Get(name) == "" {
				continue
			}
			if spread[name] == nil {
				spread[name] = make(map[string]bool)
			}
			spread[name][record.Attrs.Get(name)] = true
		}
	}
	if len(prices) == 0 {
		return dist
	}
	for name, values := range spread {
		if len(values) < 2 {
			continue
		}
		listed := make([]string, 0, len(values))
		for value := range values {
			listed = append(listed, value)
		}
		sort.Strings(listed)
		dist.Spread[name] = listed
	}
	sort.Ints(prices)
	dist.Count = len(prices)
	dist.Min, dist.Max = prices[0], prices[len(prices)-1]
	dist.P25, dist.Median, dist.P75 = percentile(prices, 25), percentile(prices, 50), percentile(prices, 75)
	dist.Buckets = bucketize(prices)
	return dist
}

// percentile returns the linearly interpolated percentile of sorted prices, the
// convention a reader expects: the median of four listings sits between the two
// middle ones rather than on the lower of them.
func percentile(sorted []int, p int) int {
	switch len(sorted) {
	case 0:
		return 0
	case 1:
		return sorted[0]
	}
	pos := float64(p) / 100 * float64(len(sorted)-1)
	lower := int(pos)
	if lower >= len(sorted)-1 {
		return sorted[len(sorted)-1]
	}
	frac := pos - float64(lower)
	return sorted[lower] + int(float64(sorted[lower+1]-sorted[lower])*frac+0.5)
}

// bucketWidths are the band sizes tried, narrowest first. Asking prices almost
// never repeat exactly -- the same phone is listed at 36200, 36500 and 36800 --
// so a tally of exact prices says nothing, and bands are what show the shape.
// A thousand is the narrowest offered because it is the granularity people
// actually haggle in.
var bucketWidths = []int{1000, 2000, 5000, 10000, 20000}

// maxBuckets caps how many bands a message will carry.
const maxBuckets = 16

func bucketize(sorted []int) []Bucket {
	width := bucketWidths[len(bucketWidths)-1]
	for _, candidate := range bucketWidths {
		// Count the bands that would actually hold something, not the bands the
		// range spans. Only occupied bands are printed, so a handful of listings
		// spread thinly across a wide range still reads as a handful of rows --
		// judging by the span alone widens the bands for a crowding that never
		// happens.
		if occupiedBands(sorted, candidate) <= maxBuckets {
			width = candidate
			break
		}
	}
	counts := make(map[int]int)
	for _, price := range sorted {
		counts[price/width*width]++
	}
	froms := make([]int, 0, len(counts))
	for from := range counts {
		froms = append(froms, from)
	}
	sort.Ints(froms)
	buckets := make([]Bucket, 0, len(froms))
	for _, from := range froms {
		buckets = append(buckets, Bucket{
			From:  from,
			To:    from + width - 1,
			Count: counts[from],
			Share: float64(counts[from]) / float64(len(sorted)),
		})
	}
	return buckets
}

func occupiedBands(sorted []int, width int) int {
	bands := make(map[int]bool, len(sorted))
	for _, price := range sorted {
		bands[price/width*width] = true
	}
	return len(bands)
}

// String renders a distribution for a chat message.
func (d Distribution) String() string {
	label := d.Kind.Label(d.Query)
	days := int(d.Window.Hours() / 24)
	if d.Count == 0 {
		return fmt.Sprintf("%s：近 %d 天沒有資料", label, days)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s（近 %d 天, %d 筆", label, days, d.Count)
	if d.Sold > 0 {
		fmt.Fprintf(&sb, ", 其中 %d 筆已售出", d.Sold)
	}
	sb.WriteString("）\n")
	fmt.Fprintf(&sb, "中位數 %s   P25 %s   P75 %s\n", myutil.Comma(d.Median), myutil.Comma(d.P25), myutil.Comma(d.P75))
	fmt.Fprintf(&sb, "區間 %s ~ %s\n", myutil.Comma(d.Min), myutil.Comma(d.Max))
	for _, name := range d.Kind.GroupBy() {
		values, ok := d.Spread[name]
		if !ok {
			continue
		}
		shown := make([]string, 0, len(values))
		for _, value := range values {
			shown = append(shown, d.Kind.AttrValue(name, value))
		}
		fmt.Fprintf(&sb, "⚠ 含多種%s：%s（指定後更準）\n",
			d.Kind.AttrLabel(name), strings.Join(shown, "、"))
	}
	for _, bucket := range d.Buckets {
		fmt.Fprintf(&sb, "%s-%s %s %.0f%% (%d 筆)\n",
			myutil.Comma(bucket.From), myutil.Comma(bucket.To),
			strings.Repeat("█", barWidth(bucket.Share)), bucket.Share*100, bucket.Count)
	}
	return strings.TrimRight(sb.String(), "\n")
}

func barWidth(share float64) int {
	width := int(share*20 + 0.5)
	if width < 1 {
		width = 1
	}
	return width
}

// DefaultWindow is the period a query covers unless asked otherwise. Second-hand
// prices drift, so a window short enough to be current beats a longer one with
// more samples: six months of a model spans its launch and its decline, and
// averaging across that describes neither.
const DefaultWindow = 30 * 24 * time.Hour

// minComparable is the smallest sample worth quoting. Below it a median says
// more about who happened to post than about the market.
const minComparable = 5

// Query summarises what a product has been asking over a window.
func Query(kind Kind, query Attrs, window time.Duration) (Distribution, error) {
	records, err := Load(time.Now().Add(-window))
	if err != nil {
		return Distribution{Kind: kind, Query: query, Window: window}, err
	}
	return Describe(kind, records, query, window), nil
}

// Compare reports how a price sits against the recent market for its product, as
// a sentence to append to an alert. It returns "" when there is nothing useful
// to say, so a caller can append it unconditionally.
func Compare(kind Kind, query Attrs, price int) string {
	if len(query) == 0 || price <= 0 {
		return ""
	}
	dist, err := Query(kind, query, DefaultWindow)
	if err != nil || dist.Count < minComparable || dist.Median <= 0 {
		return ""
	}
	days := int(DefaultWindow.Hours() / 24)
	diff := float64(price-dist.Median) / float64(dist.Median) * 100
	switch {
	case diff <= -5:
		return fmt.Sprintf("低於近 %d 天中位數 %s 約 %.0f%%（%d 筆）", days, myutil.Comma(dist.Median), -diff, dist.Count)
	case diff >= 5:
		return fmt.Sprintf("高於近 %d 天中位數 %s 約 %.0f%%（%d 筆）", days, myutil.Comma(dist.Median), diff, dist.Count)
	default:
		return fmt.Sprintf("與近 %d 天中位數 %s 相當（%d 筆）", days, myutil.Comma(dist.Median), dist.Count)
	}
}
