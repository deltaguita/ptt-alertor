package market

import (
	"regexp"
	"testing"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/price"
)

// testKind is a deliberately minimal kind: two grouping attributes, one of them
// required to identify a unit. It keeps these tests about the shared machinery
// rather than about any real product.
type testKind struct{}

func (testKind) Name() string                       { return "test" }
func (testKind) AttrSchema() map[string]interface{} { return map[string]interface{}{} }
func (testKind) PromptRules() string                { return "" }
func (testKind) TitlePattern() *regexp.Regexp       { return regexp.MustCompile(`widget`) }
func (testKind) Attrs(price.Item) (Attrs, bool)     { return nil, false }
func (testKind) ParseQuery(string) Attrs            { return nil }
func (testKind) Label(a Attrs) string               { return "widget " + a.Get("size") }
func (testKind) GroupBy() []string                  { return []string{"size", "colour"} }
func (testKind) AttrLabel(name string) string       { return "測試" + name }
func (testKind) AttrValue(_, value string) string   { return value + "!" }
func (testKind) DedupeKey(a Attrs) (string, bool) {
	if !a.Has("size", "colour") {
		return "", false
	}
	return a.Get("size") + "|" + a.Get("colour"), true
}

func rec(author, size, colour string, price, daysAgo int) Record {
	attrs := Attrs{}
	if size != "" {
		attrs["size"] = size
	}
	if colour != "" {
		attrs["colour"] = colour
	}
	return Record{
		Kind: "test", Author: author, Attrs: attrs, Price: price, PostType: "販售",
		Code:     author + "-" + time.Now().AddDate(0, 0, -daysAgo).Format("0102150405"),
		PostedAt: time.Now().AddDate(0, 0, -daysAgo),
	}
}

func TestDedupeKeepsLatestPerSellerAndSpec(t *testing.T) {
	records := []Record{
		rec("seart", "L", "red", 39300, 17),
		rec("seart", "L", "red", 40000, 3), // relist, 14 days later
		rec("other", "L", "red", 38000, 5),
	}
	got := Dedupe(testKind{}, records)
	if len(got) != 2 {
		t.Fatalf("Dedupe() kept %d records, want 2", len(got))
	}
	for _, record := range got {
		if record.Author == "seart" && record.Price != 40000 {
			t.Errorf("kept %d for a relisted seller, want the latest 40000", record.Price)
		}
	}
}

func TestDedupeLeavesUnidentifiableRecordsAlone(t *testing.T) {
	// Without every identifying attribute the kind refuses to merge, so two
	// listings by one seller stay two listings.
	records := []Record{
		rec("jkb6", "L", "", 30000, 4),
		rec("jkb6", "L", "", 31000, 3),
	}
	if got := Dedupe(testKind{}, records); len(got) != 2 {
		t.Errorf("Dedupe() merged unidentifiable listings, kept %d want 2", len(got))
	}
}

func TestDescribe(t *testing.T) {
	records := []Record{
		rec("a", "L", "red", 36000, 5),
		rec("b", "L", "red", 36500, 4),
		rec("c", "L", "red", 40500, 3),
		rec("d", "L", "red", 34000, 2),
		rec("e", "M", "red", 29000, 2), // different size
		rec("f", "L", "red", 1000, 90), // outside the window
	}
	records[3].Sold = true
	dist := Describe(testKind{}, records, Attrs{"size": "L"}, 30*24*time.Hour)

	if dist.Count != 4 {
		t.Errorf("Count = %d, want 4 (other size excluded, stale excluded)", dist.Count)
	}
	if dist.Sold != 1 {
		t.Errorf("Sold = %d, want 1", dist.Sold)
	}
	if dist.Min != 34000 || dist.Max != 40500 {
		t.Errorf("range = %d~%d, want 34000~40500", dist.Min, dist.Max)
	}
	// 34000, 36000, 36500, 40500 -> the median sits between the middle two.
	if dist.Median != 36250 {
		t.Errorf("Median = %d, want 36250", dist.Median)
	}
	total := 0
	for _, bucket := range dist.Buckets {
		total += bucket.Count
	}
	if total != dist.Count {
		t.Errorf("buckets hold %d prices, want %d", total, dist.Count)
	}
}

func TestDescribeReportsWhatAnOpenQuerySpans(t *testing.T) {
	records := []Record{
		rec("a", "L", "red", 36000, 5),
		rec("b", "L", "blue", 40500, 4),
		rec("c", "L", "red", 34000, 3),
	}
	dist := Describe(testKind{}, records, Attrs{"size": "L"}, 30*24*time.Hour)
	if got := dist.Spread["colour"]; len(got) != 2 {
		t.Errorf("Spread[colour] = %v, want both colours reported", got)
	}
	if _, reported := dist.Spread["size"]; reported {
		t.Error("Spread reported the attribute the query pinned")
	}
}

func TestDescribeExcludesWantAds(t *testing.T) {
	records := []Record{
		rec("a", "L", "red", 29000, 2),
		rec("b", "L", "red", 25000, 2),
	}
	records[1].PostType = "徵求"
	dist := Describe(testKind{}, records, Attrs{"size": "L"}, 30*24*time.Hour)
	if dist.Count != 1 || dist.Median != 29000 {
		t.Errorf("want-ad leaked in: count=%d median=%d", dist.Count, dist.Median)
	}
}

func TestDescribeIgnoresOtherKinds(t *testing.T) {
	records := []Record{rec("a", "L", "red", 29000, 2), rec("b", "L", "red", 1, 2)}
	records[1].Kind = "something-else"
	if dist := Describe(testKind{}, records, Attrs{"size": "L"}, 30*24*time.Hour); dist.Count != 1 {
		t.Errorf("Count = %d, want 1: another kind's records leaked in", dist.Count)
	}
}

func TestGroupingOfDropsDescriptiveAttributes(t *testing.T) {
	// Comparing on a descriptive attribute would match only items in identical
	// condition, which is almost never anything.
	attrs := Attrs{"size": "L", "colour": "red", "wear": "light"}
	got := GroupingOf(testKind{}, attrs)
	if len(got) != 2 || got.Get("size") != "L" || got.Get("colour") != "red" {
		t.Errorf("GroupingOf() = %v, want only the grouping attributes", got)
	}
}

func TestAttrsMatches(t *testing.T) {
	attrs := Attrs{"size": "L", "colour": "Red"}
	for _, tt := range []struct {
		query Attrs
		want  bool
	}{
		{Attrs{}, true},
		{Attrs{"size": "L"}, true},
		{Attrs{"colour": "red"}, true}, // case-insensitive
		{Attrs{"size": "M"}, false},
		{Attrs{"size": "L", "colour": "blue"}, false},
		{Attrs{"size": ""}, true}, // an empty value means "any"
	} {
		if got := attrs.Matches(tt.query); got != tt.want {
			t.Errorf("Matches(%v) = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestPercentile(t *testing.T) {
	sorted := []int{10, 20, 30, 40, 50}
	for _, tt := range []struct{ p, want int }{{0, 10}, {25, 20}, {50, 30}, {75, 40}, {100, 50}} {
		if got := percentile(sorted, tt.p); got != tt.want {
			t.Errorf("percentile(P%d) = %d, want %d", tt.p, got, tt.want)
		}
	}
	if got := percentile([]int{10, 20}, 50); got != 15 {
		t.Errorf("percentile of two values = %d, want 15", got)
	}
	if got := percentile(nil, 50); got != 0 {
		t.Errorf("percentile of nothing = %d, want 0", got)
	}
}
