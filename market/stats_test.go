package market

import (
	"testing"
	"time"
)

func rec(author, model, variant string, capacity, price int, daysAgo int) Record {
	return Record{
		Author: author, Model: model, Variant: variant, CapacityGB: capacity,
		Price: price, PostType: "販售",
		Code:     author + "-" + time.Now().AddDate(0, 0, -daysAgo).Format("0102"),
		PostedAt: time.Now().AddDate(0, 0, -daysAgo),
	}
}

func TestDedupeKeepsLatestPerSellerAndSpec(t *testing.T) {
	records := []Record{
		rec("seart", "17", "Pro Max", 256, 39300, 17),
		rec("seart", "17", "Pro Max", 256, 40000, 3), // relist, 14 days later
		rec("other", "17", "Pro Max", 256, 38000, 5),
	}
	got := Dedupe(records)
	if len(got) != 2 {
		t.Fatalf("Dedupe() kept %d records, want 2: %+v", len(got), got)
	}
	for _, record := range got {
		if record.Author == "seart" && record.Price != 40000 {
			t.Errorf("kept price %d for relisted seller, want the latest 40000", record.Price)
		}
	}
}

func TestDedupeKeepsUnknownCapacityApart(t *testing.T) {
	// Two listings by one seller with no capacity stated are not evidence of a
	// relist -- the board's ten-day rule means same-day pairs are different items.
	records := []Record{
		rec("jkb6", "17", "Pro", 0, 30000, 4),
		rec("jkb6", "17", "Pro", 0, 31000, 3),
	}
	if got := Dedupe(records); len(got) != 2 {
		t.Errorf("Dedupe() merged unknown-capacity listings, kept %d want 2", len(got))
	}
}

func TestDescribe(t *testing.T) {
	records := []Record{
		rec("a", "17", "Pro Max", 256, 36000, 5),
		rec("b", "17", "Pro Max", 256, 36500, 4),
		rec("c", "17", "Pro Max", 256, 40500, 3),
		rec("d", "17", "Pro Max", 256, 34000, 2),
		rec("e", "17", "Pro", 256, 29000, 2),     // different variant
		rec("f", "17", "Pro Max", 256, 1000, 90), // outside the window
	}
	records[3].Sold = true
	dist := Describe(records, Spec{Model: "17", Variant: "Pro Max", CapacityGB: 256}, 30*24*time.Hour)

	if dist.Count != 4 {
		t.Errorf("Count = %d, want 4 (Pro excluded, stale excluded)", dist.Count)
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

func TestDescribeExcludesWantAds(t *testing.T) {
	records := []Record{
		rec("a", "17", "Pro", 256, 29000, 2),
		rec("b", "17", "Pro", 256, 25000, 2),
	}
	records[1].PostType = "徵求"
	dist := Describe(records, Spec{Model: "17", Variant: "Pro", CapacityGB: 256}, 30*24*time.Hour)
	if dist.Count != 1 || dist.Median != 29000 {
		t.Errorf("want-ad leaked into distribution: count=%d median=%d", dist.Count, dist.Median)
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
