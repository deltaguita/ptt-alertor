package market

import (
	"testing"
	"time"
)

func TestResumeFrom(t *testing.T) {
	// Where the catch-up stopped, which is what a first backfill continues from.
	const reached = 4002
	tests := []struct {
		name     string
		stored   int
		found    bool
		wantPage int
		wantDone bool
	}{
		{"never run continues from where the catch-up stopped", 0, false, reached, false},
		{"mid backfill resumes where it stopped", 3600, true, 3600, false},
		{"finished stays finished", backfillDone, true, 0, true},
		{"a zero left by an older build restarts rather than stalls", 0, true, reached, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, done := resumeFrom(tt.stored, tt.found, reached)
			if page != tt.wantPage || done != tt.wantDone {
				t.Errorf("resumeFrom(%d, %v) = (%d, %v), want (%d, %v)",
					tt.stored, tt.found, page, done, tt.wantPage, tt.wantDone)
			}
		})
	}
}

func TestBoardsDefaultsAndTrims(t *testing.T) {
	t.Setenv("MARKET_SURVEY_BOARDS", "")
	if got := Boards(); len(got) != 1 || got[0] != "macshop" {
		t.Errorf("Boards() = %v, want the default", got)
	}
	t.Setenv("MARKET_SURVEY_BOARDS", " MacShop , nb-shopping ,, ")
	got := Boards()
	if len(got) != 2 || got[0] != "MacShop" || got[1] != "nb-shopping" {
		t.Errorf("Boards() = %v, want two trimmed names", got)
	}
}

func TestCaughtUp(t *testing.T) {
	now := time.Now()
	watermark := now.Add(-12 * time.Hour)
	page := func(oldest time.Duration) pageResult {
		return pageResult{ok: true, oldest: now.Add(-oldest), newest: now}
	}
	tests := []struct {
		name      string
		page      pageResult
		watermark time.Time
		want      bool
	}{
		{
			"page still newer than what is held, keep walking",
			page(6 * time.Hour), watermark, false,
		},
		{
			"page reaches past what is held, stop",
			page(24 * time.Hour), watermark, true,
		},
		{
			"page ends exactly at the watermark, stop",
			pageResult{ok: true, oldest: watermark, newest: now}, watermark, true,
		},
		{
			"nothing recorded yet, one page is enough",
			page(6 * time.Hour), time.Time{}, true,
		},
		{
			"unreadable page stops the walk rather than stepping over it",
			pageResult{}, watermark, true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := caughtUp(tt.page, tt.watermark); got != tt.want {
				t.Errorf("caughtUp() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCatchUpCoversAnOutage(t *testing.T) {
	// A board posting 50 articles a day fills a 20-article page in about ten
	// hours, so a two-day outage sits about five pages back. The walk has to
	// reach it; stopping at a fixed two pages would lose the difference for good.
	const perPage = 10 * time.Hour
	now := time.Now()
	watermark := now.Add(-48 * time.Hour)
	walked := 0
	for ; walked < catchUpPages; walked++ {
		page := pageResult{
			ok:     true,
			newest: now.Add(-time.Duration(walked) * perPage),
			oldest: now.Add(-time.Duration(walked+1) * perPage),
		}
		if caughtUp(page, watermark) {
			break
		}
	}
	if walked < 4 {
		t.Errorf("walked %d pages, want enough to reach a two-day gap", walked)
	}
	if walked >= catchUpPages {
		t.Errorf("walk hit the cap at %d pages without catching up", walked)
	}
}
