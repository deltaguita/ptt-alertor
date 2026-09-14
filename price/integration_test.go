package price

import (
	"os"
	"testing"

	"github.com/Ptt-Alertor/ptt-alertor/ptt/web"
)

// TestIntegrationExtract exercises the real path -- fetch a live article, ask
// Gemini to read it, cache the answer in Redis -- against the articles that
// pattern matching gets wrong. Skipped unless PRICE_INTEGRATION is set, since it
// needs network, an API key and a Redis instance.
func TestIntegrationExtract(t *testing.T) {
	if os.Getenv("PRICE_INTEGRATION") == "" {
		t.Skip("set PRICE_INTEGRATION to run")
	}
	tests := []struct {
		name      string
		code      string
		maxPrice  int
		want      Decision
		wantPrice int
	}{
		{"multi-item post, cheapest under ceiling", "M.1789351794.A.786", 5000, Notify, 1700},
		{"multi-item post, none under ceiling", "M.1789351794.A.786", 1000, Skip, 0},
		{"already sold is skipped", "M.1789321058.A.B3E", 40000, Skip, 0},
		{"single item under ceiling", "M.1789367296.A.A75", 5000, Notify, 4000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := Of(testKind{}, "MacShop", tt.code, func() (string, error) {
				a, err := web.FetchArticle("MacShop", tt.code)
				return a.Content, err
			})
			if err != nil {
				t.Fatalf("Of() error: %v", err)
			}
			got, item := Decide(info, tt.maxPrice)
			if got != tt.want || item.Price != tt.wantPrice {
				t.Errorf("Decide() = (%v, %d), want (%v, %d)\ninfo: %+v",
					got, item.Price, tt.want, tt.wantPrice, info)
			}
		})
	}
}
