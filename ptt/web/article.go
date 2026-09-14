package web

import (
	"strings"

	"golang.org/x/net/html"
)

func findTitleDiv(node *html.Node) *html.Node {
	return findDivByClassName(node, "title")
}

func findMetaDiv(node *html.Node) *html.Node {
	return findDivByClassName(node, "meta")
}

func findDateDiv(node *html.Node) *html.Node {
	return findDivByClassName(node, "date")
}

func findAuthorDiv(node *html.Node) *html.Node {
	return findDivByClassName(node, "author")
}

func findDividerDiv(node *html.Node) *html.Node {
	return findDivByClassName(node, "r-list-sep")
}

func findOgTitleMeta(node *html.Node) *html.Node {
	return findMeta(node, "og:title")
}

func findEmailProtected(node *html.Node) *html.Node {
	n := findAnchor(node)
	if n != nil {
		for _, attr := range n.Attr {
			if attr.Key == "class" && attr.Val == "__cf_email__" {
				return n
			}
		}
	}
	return nil
}

func findMainContentDiv(node *html.Node) *html.Node {
	return findDivByClassName(node, "bbs-screen bbs-content")
}

// isBodyTerminator reports whether the node marks the end of the article body.
// PTT puts the signature block (span.f2 "※ 發信站") and the comments (div.push)
// after the body, both as siblings of the body's text nodes.
func isBodyTerminator(node *html.Node) bool {
	if node.Type != html.ElementNode {
		return false
	}
	for _, attr := range node.Attr {
		if attr.Key != "class" {
			continue
		}
		if attr.Val == "f2" || attr.Val == "push" {
			return true
		}
	}
	return false
}

// getMainContent extracts the article body text from a main-content div.
// It keeps only the direct text children, which excludes the metaline divs
// (作者/看板/標題/時間) and stops at the signature block, so the comments and
// the board's boilerplate footer never reach the caller.
func getMainContent(mainContent *html.Node) string {
	var sb strings.Builder
	for child := mainContent.FirstChild; child != nil; child = child.NextSibling {
		if isBodyTerminator(child) {
			break
		}
		if child.Type == html.TextNode {
			sb.WriteString(child.Data)
		}
	}
	return strings.TrimSpace(trimSignature(sb.String()))
}

// trimSignature drops the poster's signature block. On PTT a line containing
// only "--" separates the body from the signature, so cut at the last one --
// cutting at the first would truncate bodies that use "--" as a divider.
func trimSignature(content string) string {
	if i := strings.LastIndex(content, "\n--\n"); i != -1 {
		return content[:i]
	}
	return content
}
