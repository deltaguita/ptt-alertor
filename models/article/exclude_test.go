package article

import "testing"

// Pro and Pro Max share a prefix, so telling them apart from a title needs the
// exclusion form rather than a plain keyword.
func TestMatchKeywordExcludesProMax(t *testing.T) {
	const keyword = "iPhone 17 Pro&!Max"
	tests := []struct {
		title string
		want  bool
	}{
		{"[販售] 台中 iphone 17 pro 256g 橘", true},
		{"[販售] 新竹 iPhone 17 Pro 512GB", true},
		{"[販售] 台北 iPhone 17 Pro Max 256G 橘色", false},
		{"[販售] 桃園 iPhone 17 ProMax 256 銀", false}, // no space
		{"[販售] 台中 IPHONE 17 PRO MAX 256G", false}, // upper case
		// "pm" is how some sellers write Pro Max. It is dropped, but by the
		// inclusion failing rather than the exclusion firing -- the title
		// never contains "pro" at all.
		{"[販售] 台北 iphone 17 pm 256 白色", false},
		{"[販售] 台北 iPhone 16 Pro 256G", false}, // different generation
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			if got := (Article{Title: tt.title}).MatchKeyword(keyword); got != tt.want {
				t.Errorf("MatchKeyword(%q) on %q = %v, want %v", keyword, tt.title, got, tt.want)
			}
		})
	}
}

// The exclusion is a plain substring test over the whole title, so a mention of
// the excluded word anywhere drops the article.
func TestExclusionAppliesToTheWholeTitle(t *testing.T) {
	title := "[販售] 台北 iPhone 17 Pro 256G 送 AirPods Max"
	if (Article{Title: title}).MatchKeyword("iPhone 17 Pro&!Max") {
		t.Error("expected the unrelated Max to drop this article")
	}
}
