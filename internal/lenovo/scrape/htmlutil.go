package scrape

import (
	"strings"

	"golang.org/x/net/html"
)

func hasClass(n *html.Node, class string) bool {
	for _, a := range n.Attr {
		if a.Key != "class" {
			continue
		}
		for _, tok := range strings.Fields(a.Val) {
			if tok == class {
				return true
			}
		}
	}
	return false
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textContent(n *html.Node) string {
	if n == nil {
		return ""
	}
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(textContent(c))
	}
	return strings.TrimSpace(b.String())
}

func firstDescendant(n *html.Node, tag string, pred func(*html.Node) bool) *html.Node {
	if n == nil {
		return nil
	}
	if n.Type == html.ElementNode && n.Data == tag && (pred == nil || pred(n)) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := firstDescendant(c, tag, pred); found != nil {
			return found
		}
	}
	return nil
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func collectDescendants(n *html.Node, tag string, pred func(*html.Node) bool, out *[]*html.Node) {
	if n == nil {
		return
	}
	if n.Type == html.ElementNode && n.Data == tag && (pred == nil || pred(n)) {
		*out = append(*out, n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectDescendants(c, tag, pred, out)
	}
}
