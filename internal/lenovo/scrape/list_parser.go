package scrape

import (
	"net/url"
	"strconv"
	"strings"

	"ecommerce-crawler-api/internal/lenovo/model"

	"golang.org/x/net/html"
)

// NextListingPageURL retorna o href absoluto da página seguinte ou "".
func NextListingPageURL(doc *html.Node, currentAbs string) (string, error) {
	var href string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" && hasClass(n, "page-link") && hasClass(n, "next") {
			for _, a := range n.Attr {
				if a.Key == "href" && strings.TrimSpace(a.Val) != "" {
					href = strings.TrimSpace(a.Val)
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if href == "" {
		return "", nil
	}
	base, err := url.Parse(currentAbs)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(href)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}

// ParseListingProducts extrai todos os cards de produto da listagem de laptops.
func ParseListingProducts(doc *html.Node) []model.ListingsSummary {
	var out []model.ListingsSummary
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" && hasClass(n, "thumbnail") && hasClass(n, "card") {
			if p := parseProductCard(n); p.RelativeURL != "" {
				out = append(out, p)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

func parseProductCard(root *html.Node) model.ListingsSummary {
	var s model.ListingsSummary
	titleA := firstDescendant(root, "a", func(n *html.Node) bool { return hasClass(n, "title") })
	if titleA != nil {
		s.RelativeURL = attr(titleA, "href")
		if t := attr(titleA, "title"); t != "" {
			s.Name = strings.TrimSpace(t)
		} else {
			s.Name = strings.TrimSpace(textContent(titleA))
		}
	}
	priceSpan := firstDescendant(root, "span", func(n *html.Node) bool { return attr(n, "itemprop") == "price" })
	if priceSpan != nil {
		s.Price = strings.TrimSpace(textContent(priceSpan))
	}
	descP := firstDescendant(root, "p", func(n *html.Node) bool { return hasClass(n, "description") })
	if descP != nil {
		s.Description = strings.TrimSpace(textContent(descP))
	}
	revSpan := firstDescendant(root, "span", func(n *html.Node) bool { return attr(n, "itemprop") == "reviewCount" })
	if revSpan != nil {
		s.ReviewCount, _ = strconv.Atoi(strings.TrimSpace(textContent(revSpan)))
	}
	ratingP := firstDescendant(root, "p", func(n *html.Node) bool { return attr(n, "data-rating") != "" })
	if ratingP != nil {
		s.Rating, _ = strconv.Atoi(attr(ratingP, "data-rating"))
	}
	return s
}

// IsLenovoBrand retorna true se nome ou descrição indicam marca Lenovo no catálogo de teste.
func IsLenovoBrand(name, description string) bool {
	hay := strings.ToLower(name + " " + description)
	return strings.Contains(hay, "lenovo")
}
