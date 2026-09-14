package price

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Ptt-Alertor/ptt-alertor/ptt/web"
)

type phoneKind struct{}

func (phoneKind) Name() string { return "iphone-probe" }
func (phoneKind) AttrSchema() map[string]interface{} {
	return map[string]interface{}{
		"model":    map[string]interface{}{"type": "string"},
		"variant":  map[string]interface{}{"type": "string", "enum": []string{"Pro Max", "Pro", "Plus", "無"}},
		"capacity": map[string]interface{}{"type": "string"},
		"battery":  map[string]interface{}{"type": "string"},
	}
}
func (phoneKind) PromptRules() string {
	return `7. 商品是 iPhone 手機本體時才填寫 attrs。
   - model：世代數字。capacity：容量 GB 數字串。battery：電池健康度數字。
`
}

func TestIntegrationAttributes(t *testing.T) {
	if os.Getenv("PRICE_INTEGRATION") == "" {
		t.Skip("set PRICE_INTEGRATION to run")
	}
	for _, code := range []string{"M.1789367105.A.4F7", "M.1789321058.A.B3E"} {
		info, err := Of(phoneKind{}, "MacShop", code, func() (string, error) {
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
