package market

import (
	"regexp"
	"testing"
)

func TestResumeFrom(t *testing.T) {
	const newest = 4004
	tests := []struct {
		name     string
		stored   int
		found    bool
		wantPage int
		wantDone bool
	}{
		{"never run starts behind the newest pages", 0, false, newest - 2, false},
		{"mid backfill resumes where it stopped", 3600, true, 3600, false},
		{"finished stays finished", backfillDone, true, 0, true},
		{"a zero left by an older build restarts rather than stalls", 0, true, newest - 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, done := resumeFrom(tt.stored, tt.found, newest)
			if page != tt.wantPage || done != tt.wantDone {
				t.Errorf("resumeFrom(%d, %v) = (%d, %v), want (%d, %v)",
					tt.stored, tt.found, page, done, tt.wantPage, tt.wantDone)
			}
		})
	}
}

func TestDefaultPatternMatchesPhonesNotAccessoriesByName(t *testing.T) {
	pattern := regexp.MustCompile(defaultPattern)
	for _, title := range []string{
		"[販售] 台北 iPhone 17 Pro Max 256G 橘色",
		"[販售] 新竹 iphone 16 pro 128g",
		"[販售] 台中 iPhone 19 Pro", // a generation that does not exist yet
	} {
		if !pattern.MatchString(title) {
			t.Errorf("pattern missed %q", title)
		}
	}
	for _, title := range []string{
		"[販售] 全國 AirPods 4 (ANC)",
		"[販售] 桃園 Mac mini M4 16/512",
	} {
		if pattern.MatchString(title) {
			t.Errorf("pattern matched non-phone %q", title)
		}
	}
}

func TestEnvPatternFallsBackOnGarbage(t *testing.T) {
	t.Setenv("MARKET_SURVEY_PATTERN", "([unclosed")
	if got := envPattern("MARKET_SURVEY_PATTERN").String(); got != defaultPattern {
		t.Errorf("envPattern() = %q, want the default after an invalid pattern", got)
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
