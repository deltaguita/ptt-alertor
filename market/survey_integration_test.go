package market

import (
	"os"
	"testing"
	"time"
)

// TestIntegrationSurvey runs a small survey against the live board and reports
// what it recorded. Skipped unless MARKET_INTEGRATION is set.
func TestIntegrationSurvey(t *testing.T) {
	if os.Getenv("MARKET_INTEGRATION") == "" {
		t.Skip("set MARKET_INTEGRATION to run")
	}
	survey := NewSurvey("MacShop")
	survey.Budget = 12
	survey.Pause = 1500 * time.Millisecond

	start := time.Now()
	survey.Run()
	t.Logf("survey finished in %s", time.Since(start).Round(time.Second))

	records, err := Load(time.Now().Add(-DefaultWindow))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 {
		t.Fatal("survey recorded nothing")
	}
	t.Logf("recorded %d rows (%d after dedupe)", len(records), len(Dedupe(records)))
	for _, r := range records {
		t.Logf("  %s %-14s %-22s %6d  電池%-4d sold=%v",
			r.PostedAt.Format("01/02"), r.Author, r.Spec(), r.Price, r.BatteryHealth, r.Sold)
	}
	for _, spec := range []Spec{
		{Model: "17", Variant: VariantProMax},
		{Model: "17", Variant: VariantPro},
	} {
		t.Logf("\n%s", Describe(records, spec, DefaultWindow))
	}
}
