package web

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

const articleBody = `<div id="main-content" class="bbs-screen bbs-content">` +
	`<div class="article-metaline"><span class="article-meta-tag">作者</span>` +
	`<span class="article-meta-value">someone (nobody)</span></div>` +
	`<div class="article-metaline"><span class="article-meta-tag">標題</span>` +
	`<span class="article-meta-value">[販售] 台北 測試商品</span></div>` +
	"[型號]\n測試商品\n\n[售價]\n12000\n--\n寄自 BePTT\n" +
	`<span class="f2">※ 發信站: 批踢踢實業坊(ptt.cc)</span>` +
	`<div class="push"><span class="f3 push-content">: 推一個</span></div>` +
	`</div>`

func parseContent(t *testing.T, doc string) string {
	t.Helper()
	nodes, err := html.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	found := findNodes(nodes, findMainContentDiv)
	if len(found) == 0 {
		t.Fatal("main-content div not found")
	}
	return getMainContent(found[0])
}

func TestGetMainContent(t *testing.T) {
	got := parseContent(t, articleBody)

	if !strings.Contains(got, "[售價]\n12000") {
		t.Errorf("body text missing, got:\n%s", got)
	}
	for _, unwanted := range []string{"作者", "someone", "[販售] 台北 測試商品"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("metaline %q leaked into content:\n%s", unwanted, got)
		}
	}
	if strings.Contains(got, "※ 發信站") {
		t.Errorf("signature block leaked into content:\n%s", got)
	}
	if strings.Contains(got, "推一個") {
		t.Errorf("comments leaked into content:\n%s", got)
	}
	if strings.Contains(got, "寄自 BePTT") {
		t.Errorf("poster signature leaked into content:\n%s", got)
	}
}

func TestTrimSignatureKeepsBodyDivider(t *testing.T) {
	content := "第一段\n--\n第二段\n--\n簽名檔"
	if got, want := trimSignature(content), "第一段\n--\n第二段"; got != want {
		t.Errorf("trimSignature() = %q, want %q", got, want)
	}
}
