package article

import "testing"

func TestParseCode(t *testing.T) {
	tests := []struct{ link, want string }{
		{"https://www.ptt.cc/bbs/MacShop/M.1789367105.A.4F7.html", "M.1789367105.A.4F7"},
		{"https://www.ptt.cc/bbs/EZsoft/G.1497363598.A.74E.html", "G.1497363598.A.74E"},
		{"", ""},
		{"https://www.ptt.cc/bbs/MacShop/index.html", ""},
	}
	for _, tt := range tests {
		if got := (Article{Link: tt.link}).ParseCode(); got != tt.want {
			t.Errorf("ParseCode(%q) = %q, want %q", tt.link, got, tt.want)
		}
	}
}
