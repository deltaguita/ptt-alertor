package market

import (
	"strings"
	"testing"
	"time"
)

func dayRec(author string, price, daysAgo int) Record {
	return Record{
		Kind: "test", Author: author, Price: price, PostType: "販售",
		Attrs:    Attrs{"size": "L", "colour": author},
		Code:     author,
		PostedAt: time.Now().AddDate(0, 0, -daysAgo),
	}
}

func TestDailyFillsGapsWithoutInventingPrices(t *testing.T) {
	records := []Record{
		dayRec("a", 36000, 4),
		dayRec("b", 34000, 2),
		dayRec("c", 35000, 2),
	}
	days := Daily(testKind{}, records, Attrs{"size": "L"}, 30*24*time.Hour)
	if len(days) != 3 {
		t.Fatalf("got %d days, want 3 spanning the first and last listing", len(days))
	}
	if days[0].Mean != 36000 || days[0].Count != 1 {
		t.Errorf("first day = %+v", days[0])
	}
	// The middle day has no listings. It must be present but empty: plotting a
	// zero would draw a crash that never happened.
	if days[1].Count != 0 || days[1].Mean != 0 {
		t.Errorf("empty day = %+v, want a gap", days[1])
	}
	if days[2].Mean != 34500 || days[2].Count != 2 {
		t.Errorf("last day = %+v, want the mean of two listings", days[2])
	}
}

func TestDailyExcludesWantAds(t *testing.T) {
	records := []Record{dayRec("a", 36000, 1), dayRec("b", 10000, 1)}
	records[1].PostType = "徵求"
	days := Daily(testKind{}, records, Attrs{"size": "L"}, 30*24*time.Hour)
	if len(days) != 1 || days[0].Mean != 36000 {
		t.Errorf("want-ad leaked into the daily mean: %+v", days)
	}
}

func TestChartMarksThinDaysDifferently(t *testing.T) {
	days := []Day{
		{Date: time.Now().AddDate(0, 0, -2), Mean: 36000, Count: 5},
		{Date: time.Now().AddDate(0, 0, -1), Mean: 34000, Count: 1},
	}
	chart := Chart(days, "測試")
	if !strings.Contains(chart, markSolid) {
		t.Error("a day with enough listings is not marked as solid")
	}
	if !strings.Contains(chart, markThin) {
		t.Error("a day resting on one listing is not marked as thin")
	}
}

func TestChartNeedsTwoDaysToBeWorthDrawing(t *testing.T) {
	days := []Day{{Date: time.Now(), Mean: 36000, Count: 5}}
	if got := Chart(days, "測試"); got != "" {
		t.Errorf("drew a chart from a single point:\n%s", got)
	}
}

func TestChartShowsCountsAndDates(t *testing.T) {
	days := []Day{
		{Date: time.Date(2026, 9, 3, 0, 0, 0, 0, time.Local), Mean: 36000, Count: 5},
		{Date: time.Date(2026, 9, 4, 0, 0, 0, 0, time.Local), Count: 0},
		{Date: time.Date(2026, 9, 5, 0, 0, 0, 0, time.Local), Mean: 34000, Count: 2},
	}
	chart := Chart(days, "測試")
	if !strings.Contains(chart, "9/3") {
		t.Errorf("the first date is not labelled:\n%s", chart)
	}
	if !strings.Contains(chart, "每日筆數") {
		t.Errorf("the counts are missing:\n%s", chart)
	}
	if !strings.Contains(chart, "·") {
		t.Errorf("the empty day is not shown as a gap:\n%s", chart)
	}
}
