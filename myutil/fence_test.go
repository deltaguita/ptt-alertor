package myutil

import (
	"strings"
	"testing"
)

func TestFenceToHTMLWrapsBlocks(t *testing.T) {
	text := "標題\n" + Fence + "\n 36,000 ┤●\n" + Fence + "\n說明"
	got, ok := FenceToHTML(text)
	if !ok {
		t.Fatal("a balanced fence was not recognised")
	}
	if !strings.Contains(got, "<pre> 36,000 ┤●</pre>") {
		t.Errorf("the chart is not preformatted:\n%s", got)
	}
	if strings.Contains(got, Fence) {
		t.Errorf("the fences leaked into the message:\n%s", got)
	}
	if !strings.Contains(got, "標題") || !strings.Contains(got, "說明") {
		t.Errorf("the surrounding text was lost:\n%s", got)
	}
}

func TestFenceToHTMLLeavesPlainTextAlone(t *testing.T) {
	if _, ok := FenceToHTML("沒有圖表的一般訊息"); ok {
		t.Error("plain text was treated as containing a block")
	}
}

func TestFenceToHTMLGivesUpOnAnUnbalancedFence(t *testing.T) {
	// What a message split across the length limit leaves behind.
	if _, ok := FenceToHTML("標題\n" + Fence + "\n 36,000 ┤●"); ok {
		t.Error("an unbalanced fence was formatted anyway")
	}
}

func TestFenceToHTMLEscapesMarkup(t *testing.T) {
	got, ok := FenceToHTML("a < b\n" + Fence + "\nx & y\n" + Fence + "\n")
	if !ok {
		t.Fatal("not recognised")
	}
	if !strings.Contains(got, "a &lt; b") || !strings.Contains(got, "x &amp; y") {
		t.Errorf("markup characters were not escaped:\n%s", got)
	}
}
