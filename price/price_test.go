package price

import (
	"strconv"
	"testing"
	"time"
)

// testKind asks for nothing beyond a price, which is all these tests need.
type testKind struct{}

func (testKind) Name() string                       { return "test" }
func (testKind) AttrSchema() map[string]interface{} { return map[string]interface{}{} }
func (testKind) PromptRules() string                { return "" }

func saleInfo(prices ...int) Info {
	items := make([]Item, 0, len(prices))
	for _, p := range prices {
		items = append(items, Item{Name: "item", Price: p})
	}
	return Info{PostType: PostTypeSale, Items: items, Confidence: ConfidenceHigh}
}

func TestDecide(t *testing.T) {
	tests := []struct {
		name     string
		info     Info
		maxPrice int
		want     Decision
		wantAt   int
	}{
		{"under ceiling", saleInfo(32000), 35000, Notify, 32000},
		{"on ceiling", saleInfo(35000), 35000, Notify, 35000},
		{"over ceiling", saleInfo(41500), 35000, Skip, 0},
		{"cheapest of several matches", saleInfo(20000, 3500, 1700), 5000, Notify, 1700},
		{"one of several under ceiling", saleInfo(41500, 30000), 35000, Notify, 30000},
		{
			"sold is skipped even when cheap",
			Info{PostType: PostTypeSale, IsSold: true, Items: []Item{{Price: 100}}, Confidence: ConfidenceHigh},
			35000, Skip, 0,
		},
		{
			"want-ad is skipped",
			Info{PostType: PostTypeWanted, Items: []Item{{Price: 5500}}, Confidence: ConfidenceHigh},
			35000, Skip, 0,
		},
		{
			"low confidence still notifies",
			Info{PostType: PostTypeSale, Items: []Item{{Price: 32000}}, Confidence: ConfidenceLow},
			35000, NotifyUnverified, 0,
		},
		{
			"no items still notifies",
			Info{PostType: PostTypeSale, Confidence: ConfidenceHigh},
			35000, NotifyUnverified, 0,
		},
		{"non-positive price ignored", saleInfo(0, 32000), 35000, Notify, 32000},
		{"only non-positive prices", saleInfo(0), 35000, Skip, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, item := Decide(tt.info, tt.maxPrice)
			if got != tt.want || item.Price != tt.wantAt {
				t.Errorf("Decide() = (%v, %d), want (%v, %d)", got, item.Price, tt.want, tt.wantAt)
			}
		})
	}
}

func TestTTLFor(t *testing.T) {
	recent := time.Now().Add(-2 * time.Hour).Unix()
	settled := time.Now().Add(-30 * 24 * time.Hour).Unix()
	tests := []struct {
		name string
		code string
		want time.Duration
	}{
		{"still editable", "M." + strconv.FormatInt(recent, 10) + ".A.4F7", freshTTL},
		{"settled", "M." + strconv.FormatInt(settled, 10) + ".A.4F7", settledTTL},
		{"unparsable code is treated as fresh", "not-a-code", freshTTL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ttlFor(tt.code); got != tt.want {
				t.Errorf("ttlFor(%q) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}
