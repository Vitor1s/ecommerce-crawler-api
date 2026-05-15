package scrape

import (
	"path"
	"strconv"
	"strings"

	"ecommerce-crawler-api/internal/lenovo/model"

	"golang.org/x/net/html"
)

// ParseProductPage extrai campos da página individual do produto (card principal).
func ParseProductPage(doc *html.Node) model.LenovoLaptop {
	var p model.LenovoLaptop
	p.Currency = "USD"

	card := firstDescendant(doc, "div", func(n *html.Node) bool {
		return hasClass(n, "thumbnail") && hasClass(n, "card") && hasAttr(n, "itemscope")
	})
	if card == nil {
		card = firstDescendant(doc, "div", func(n *html.Node) bool {
			return hasClass(n, "thumbnail") && hasClass(n, "card")
		})
	}
	if card == nil {
		return p
	}

	if img := firstDescendant(card, "img", func(n *html.Node) bool { return hasClass(n, "image") || attr(n, "itemprop") == "image" }); img != nil {
		p.ImageURL = strings.TrimSpace(attr(img, "src"))
	}

	if priceSpan := firstDescendant(card, "span", func(n *html.Node) bool { return attr(n, "itemprop") == "price" }); priceSpan != nil {
		p.Price = strings.TrimSpace(textContent(priceSpan))
		p.PriceUSD = ParseUSDPrice(p.Price)
	}

	if title := firstDescendant(card, "h4", func(n *html.Node) bool { return hasClass(n, "title") }); title != nil {
		p.Name = strings.TrimSpace(textContent(title))
	}

	if desc := firstDescendant(card, "p", func(n *html.Node) bool { return hasClass(n, "description") }); desc != nil {
		p.Description = strings.TrimSpace(textContent(desc))
	}

	if revSpan := firstDescendant(card, "span", func(n *html.Node) bool { return attr(n, "itemprop") == "reviewCount" }); revSpan != nil {
		p.ReviewCount, _ = strconv.Atoi(strings.TrimSpace(textContent(revSpan)))
	}

	ratings := firstDescendant(card, "div", func(n *html.Node) bool { return hasClass(n, "ratings") })
	if ratings != nil {
		var stars []*html.Node
		collectDescendants(ratings, "span", func(n *html.Node) bool {
			return hasClass(n, "ws-icon-star")
		}, &stars)
		if len(stars) > 0 {
			p.Rating = len(stars)
		} else if rp := firstDescendant(ratings, "p", func(n *html.Node) bool { return attr(n, "data-rating") != "" }); rp != nil {
			p.Rating, _ = strconv.Atoi(attr(rp, "data-rating"))
		}
	}

	var sw []*html.Node
	collectDescendants(card, "button", func(n *html.Node) bool { return hasClass(n, "swatch") }, &sw)
	for _, b := range sw {
		v := strings.TrimSpace(attr(b, "value"))
		if v == "" {
			continue
		}
		gb, err := strconv.Atoi(v)
		if err != nil {
			continue
		}
		p.HDDOptionsGB = append(p.HDDOptionsGB, gb)
		if hasClass(b, "active") {
			x := gb
			p.HDDSelectedGB = &x
		}
	}

	return p
}

// ParseUSDPrice interpreta preços no formato "$123.45".
func ParseUSDPrice(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "$")
	s = strings.ReplaceAll(s, ",", "")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// ProductIDFromPath retorna o segmento numérico de .../product/87.
func ProductIDFromPath(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	base := path.Base(u)
	if base == "." || base == "/" {
		return ""
	}
	return base
}
