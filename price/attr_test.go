package price

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Ptt-Alertor/ptt-alertor/ptt/web"
)

func TestIntegrationAttributes(t *testing.T) {
	if os.Getenv("PRICE_INTEGRATION") == "" {
		t.Skip("set PRICE_INTEGRATION to run")
	}
	for _, code := range []string{
		"M.1789367105.A.4F7", // 17 Pro Max 256G 橘, 電池100%, 保固2027/12/12
		"M.1789321058.A.B3E", // 17 Pro Max 256GB 已售出, 電池92%
		"M.1789351794.A.786", // 多商品: iPad + Watch + Pencil
	} {
		info, err := Of("MacShop", code, func() (string, error) {
			a, err := web.FetchArticle("MacShop", code)
			return a.Content, err
		})
		if err != nil {
			t.Fatalf("%s: %v", code, err)
		}
		out, _ := json.MarshalIndent(info, "", "  ")
		t.Logf("%s ->\n%s", code, out)
	}
}
