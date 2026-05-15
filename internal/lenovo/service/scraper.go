package service

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"ecommerce-crawler-api/internal/lenovo/model"
	"ecommerce-crawler-api/internal/lenovo/scrape"
)

const (
	siteOrigin   = "https://webscraper.io"
	laptopsStart = siteOrigin + "/test-sites/e-commerce/static/computers/laptops"
	politeness   = 100 * time.Millisecond
)

// Scraper orquestra listagem + páginas de produto.
type Scraper struct {
	HTTPClient *http.Client
}

func (s *Scraper) client() *http.Client {
	if s.HTTPClient != nil {
		return s.HTTPClient
	}
	return nil
}

func resolveRef(origin, ref string) string {
	base, err := url.Parse(origin)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return base.ResolveReference(r).String()
}

func absImage(origin, src string) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		return src
	}
	return resolveRef(origin, src)
}

func sleepPolite(ctx context.Context) error {
	t := time.NewTimer(politeness)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// FetchLenovoLaptopsSorted percorre todas as páginas de laptops, filtra Lenovo,
// visita cada página de produto e devolve a lista ordenada por preço (USD) crescente.
func (s *Scraper) FetchLenovoLaptopsSorted(ctx context.Context) ([]model.LenovoLaptop, error) {
	client := s.client()
	var allListing []model.ListingsSummary
	seenPages := map[string]struct{}{}
	cur := laptopsStart

	for cur != "" {
		if _, ok := seenPages[cur]; ok {
			break
		}
		seenPages[cur] = struct{}{}

		doc, err := scrape.FetchHTML(ctx, client, cur)
		if err != nil {
			return nil, err
		}
		allListing = append(allListing, scrape.ParseListingProducts(doc)...)

		next, err := scrape.NextListingPageURL(doc, cur)
		if err != nil {
			return nil, err
		}
		if next == "" || next == cur {
			break
		}
		cur = next
		if err := sleepPolite(ctx); err != nil {
			return nil, err
		}
	}

	byURL := map[string]model.ListingsSummary{}
	for _, it := range allListing {
		if !scrape.IsLenovoBrand(it.Name, it.Description) {
			continue
		}
		abs := resolveRef(siteOrigin, it.RelativeURL)
		if _, exists := byURL[abs]; !exists {
			byURL[abs] = it
		}
	}

	out := make([]model.LenovoLaptop, 0, len(byURL))
	for abs, summary := range byURL {
		if err := sleepPolite(ctx); err != nil {
			return nil, err
		}
		doc, err := scrape.FetchHTML(ctx, client, abs)
		if err != nil {
			return nil, err
		}
		p := scrape.ParseProductPage(doc)
		p.ProductURL = abs
		p.ProductID = scrape.ProductIDFromPath(abs)
		p.ImageURL = absImage(siteOrigin, p.ImageURL)
		if p.Name == "" {
			p.Name = summary.Name
		}
		if p.Price == "" {
			p.Price = summary.Price
			p.PriceUSD = scrape.ParseUSDPrice(summary.Price)
		}
		if p.Description == "" {
			p.Description = summary.Description
		}
		if p.ReviewCount == 0 {
			p.ReviewCount = summary.ReviewCount
		}
		if p.Rating == 0 {
			p.Rating = summary.Rating
		}
		if p.Currency == "" {
			p.Currency = "USD"
		}
		// O catálogo de testes às vezes diverge entre card e PDP; exige "Lenovo" na página do produto.
		if !scrape.IsLenovoBrand(p.Name, p.Description) {
			continue
		}
		out = append(out, p)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].PriceUSD == out[j].PriceUSD {
			return out[i].ProductURL < out[j].ProductURL
		}
		return out[i].PriceUSD < out[j].PriceUSD
	})
	return out, nil
}
