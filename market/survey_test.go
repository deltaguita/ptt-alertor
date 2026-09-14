package market

import "testing"

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
