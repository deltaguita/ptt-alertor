package market

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/myutil"
)

// Day is one day's worth of asking prices for a product.
type Day struct {
	Date  time.Time
	Mean  int
	Count int
}

// thinDay is the number of listings below which a day's mean says more about
// who happened to post than about the market. Such days are still plotted --
// they are real observations -- but marked so a reader can discount them.
const thinDay = 3

const (
	markSolid = "●"
	markThin  = "○"
)

// Daily returns one point per day in the window, oldest first, including days
// with no listings so that gaps in the board's activity stay visible. Want-ads
// are excluded, as they are from every other statistic here.
func Daily(kind Kind, records []Record, query Attrs, window time.Duration) []Day {
	cutoff := time.Now().Add(-window)
	sums := make(map[string]int)
	counts := make(map[string]int)
	for _, record := range Dedupe(kind, records) {
		if record.Kind != kind.Name() || record.PostType == "徵求" || record.Price <= 0 {
			continue
		}
		if record.PostedAt.Before(cutoff) || !record.Attrs.Matches(query) {
			continue
		}
		day := record.PostedAt.Format("2006-01-02")
		sums[day] += record.Price
		counts[day]++
	}
	if len(counts) == 0 {
		return nil
	}
	dates := make([]string, 0, len(counts))
	for day := range counts {
		dates = append(dates, day)
	}
	sort.Strings(dates)
	first, _ := time.ParseInLocation("2006-01-02", dates[0], time.Local)
	last, _ := time.ParseInLocation("2006-01-02", dates[len(dates)-1], time.Local)

	days := make([]Day, 0)
	for at := first; !at.After(last); at = at.AddDate(0, 0, 1) {
		key := at.Format("2006-01-02")
		point := Day{Date: at, Count: counts[key]}
		if point.Count > 0 {
			point.Mean = sums[key] / point.Count
		}
		days = append(days, point)
	}
	return days
}

// chartHeight is the number of price rows. Enough to show a trend, few enough
// to read on a phone.
const chartHeight = 7

// chartBudget is the widest plot area that still reads without wrapping in a
// chat message.
const chartBudget = 44

// Chart draws daily means as a plain-text scatter, to be shown in a monospaced
// block. A picture would read better, but it would need a drawing library and a
// font with Chinese glyphs on a machine that has neither, and text survives
// being quoted, copied and searched.
func Chart(days []Day, title string) string {
	withData := make([]Day, 0, len(days))
	for _, day := range days {
		if day.Count > 0 {
			withData = append(withData, day)
		}
	}
	if len(withData) < 2 {
		return ""
	}

	low, high := withData[0].Mean, withData[0].Mean
	for _, day := range withData {
		if day.Mean < low {
			low = day.Mean
		}
		if day.Mean > high {
			high = day.Mean
		}
	}
	if high == low {
		high = low + 1
	}

	step := chartBudget / len(days)
	if step < 1 {
		step = 1
	}
	if step > 2 {
		step = 2
	}

	var sb strings.Builder
	sb.WriteString(title + "\n\n")
	for row := 0; row < chartHeight; row++ {
		price := high - (high-low)*row/(chartHeight-1)
		fmt.Fprintf(&sb, "%8s ┤", myutil.Comma(roundTo(price, 100)))
		line := ""
		for _, day := range days {
			cell := " "
			if day.Count > 0 && rowOf(day.Mean, low, high) == row {
				cell = markSolid
				if day.Count < thinDay {
					cell = markThin
				}
			}
			line += cell + strings.Repeat(" ", step-1)
		}
		sb.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	sb.WriteString("         └" + strings.Repeat("─", len(days)*step) + "\n")
	sb.WriteString("          " + dateAxis(days, step) + "\n")
	sb.WriteString("\n每日筆數 " + countRow(days, step) + "\n")
	sb.WriteString(markSolid + " " + fmt.Sprintf("%d 筆以上", thinDay) + "　" + markThin + " 僅 1-2 筆（參考性低）")
	return sb.String()
}

func rowOf(price, low, high int) int {
	return int(float64(high-price)/float64(high-low)*float64(chartHeight-1) + 0.5)
}

func roundTo(value, to int) int {
	return (value + to/2) / to * to
}

// dateAxis labels as many days as fit without the labels running into one
// another.
func dateAxis(days []Day, step int) string {
	axis := make([]byte, len(days)*step+6)
	for i := range axis {
		axis[i] = ' '
	}
	nextFree := 0
	for i, day := range days {
		label := fmt.Sprintf("%d/%d", int(day.Date.Month()), day.Date.Day())
		at := i * step
		if at < nextFree || at+len(label) > len(axis) {
			continue
		}
		copy(axis[at:], label)
		nextFree = at + len(label) + 1
	}
	return strings.TrimRight(string(axis), " ")
}

func countRow(days []Day, step int) string {
	var sb strings.Builder
	for _, day := range days {
		switch {
		case day.Count == 0:
			sb.WriteString("·")
		case day.Count > 9:
			sb.WriteString("+")
		default:
			fmt.Fprintf(&sb, "%d", day.Count)
		}
		sb.WriteString(strings.Repeat(" ", step-1))
	}
	return strings.TrimRight(sb.String(), " ")
}
