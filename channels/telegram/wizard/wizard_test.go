package wizard

import (
	"strings"
	"testing"
)

func TestCommand(t *testing.T) {
	tests := []struct {
		name string
		w    Wizard
		want string
	}{
		{
			name: "board, keyword and ceiling",
			w:    Wizard{Board: "macshop", Keyword: "iPhone 17 Pro", Price: 35000},
			want: "新增售價 macshop iPhone 17 Pro 35000",
		},
		{
			// The whole point of the exclusion button: Pro and Pro Max share a
			// prefix, and this is the syntax nobody should have to know.
			name: "an exclusion becomes the &! form",
			w:    Wizard{Board: "macshop", Keyword: "iPhone 17 Pro", Exclude: "Max", Price: 35000},
			want: "新增售價 macshop iPhone 17 Pro&!Max 35000",
		},
		{
			name: "no ceiling falls back to a plain keyword",
			w:    Wizard{Board: "macshop", Keyword: "AirPods Pro 3"},
			want: "新增 macshop AirPods Pro 3",
		},
		{
			name: "no ceiling but an exclusion",
			w:    Wizard{Board: "macshop", Keyword: "iPhone 17 Pro", Exclude: "Max"},
			want: "新增 macshop iPhone 17 Pro&!Max",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.w.Command(); got != tt.want {
				t.Errorf("Command() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummaryIsWordsNotSyntax(t *testing.T) {
	w := Wizard{Board: "macshop", Keyword: "iPhone 17 Pro", Exclude: "Max", Price: 35000}
	summary := w.Summary()
	for _, want := range []string{"macshop", "iPhone 17 Pro", "Max", "35000"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary is missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "&!") {
		t.Errorf("summary leaks the syntax it exists to hide:\n%s", summary)
	}
}

func TestSummarySaysWhenThereIsNoCeiling(t *testing.T) {
	w := Wizard{Board: "macshop", Keyword: "AirPods"}
	if !strings.Contains(w.Summary(), "不限") {
		t.Errorf("a subscription without a ceiling does not say so:\n%s", w.Summary())
	}
}

func TestAwaitsText(t *testing.T) {
	for _, tt := range []struct {
		step string
		want bool
	}{
		{StepKeyword, true},
		{StepBoardTyping, true},
		{StepExcludeTyping, true},
		{StepPriceTyping, true},
		{StepBoard, false},
		{StepExclude, false},
		{StepPrice, false},
		{StepConfirm, false},
	} {
		if got := (&Wizard{Step: tt.step}).AwaitsText(); got != tt.want {
			t.Errorf("AwaitsText(%s) = %v, want %v", tt.step, got, tt.want)
		}
	}
}

func TestSuggestsProMax(t *testing.T) {
	for _, tt := range []struct {
		keyword string
		want    bool
	}{
		{"iPhone 17 Pro", true},
		{"17 pro", true},
		{"iPhone 17 Pro Max", false}, // already asking for Max
		{"AirPods", false},
		{"MacBook Air", false},
	} {
		if got := SuggestsProMax(tt.keyword); got != tt.want {
			t.Errorf("SuggestsProMax(%q) = %v, want %v", tt.keyword, got, tt.want)
		}
	}
}
