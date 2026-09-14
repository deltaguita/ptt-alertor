package iphone

import (
	"os"
	"testing"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/market"
)

// TestIntegrationSurvey runs a small survey against the live board and reports
// what it recorded. Skipped unless MARKET_INTEGRATION is set, since it needs
// network, an API key and a Redis instance.
func TestIntegrationSurvey(t *testing.T) {
	if os.Getenv("MARKET_INTEGRATION") == "" {
		t.Skip("set MARKET_INTEGRATION to run")
	}
	survey := market.NewSurvey("MacShop")
	survey.Budget = 12
	survey.Pause = 1500 * time.Millisecond

	start := time.Now()
	survey.Run()
	t.Logf("survey finished in %s", time.Since(start).Round(time.Second))

	records, err := market.Load(time.Now().Add(-market.DefaultWindow))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 {
		t.Fatal("survey recorded nothing")
	}
	t.Logf("recorded %d rows (%d after dedupe)", len(records), len(market.Dedupe(kind, records)))
	for _, r := range records {
		t.Logf("  %s %-14s %-24s %6d  電池%-4s sold=%v",
			r.PostedAt.Format("01/02"), r.Author, kind.Label(r.Attrs),
			r.Price, r.Attrs.Get(AttrBattery), r.Sold)
	}
	for _, query := range []market.Attrs{
		{AttrModel: "17", AttrVariant: VariantProMax},
		{AttrModel: "17", AttrVariant: VariantPro},
	} {
		t.Logf("\n%s", market.Describe(kind, records, query, market.DefaultWindow))
	}
}
