// Crawler books.toscrape.com — coleta o catálogo e exporta books.json (ver README.md nesta pasta).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	httpTimeout      = 30 * time.Second
	maxBodyBytes     = 5 << 20
	errSnippetLen    = 200
	defaultJSONPath  = "books.json"
	maxFetchAttempts = 3
	initialBackoff   = 300 * time.Millisecond
	maxBackoff       = 15 * time.Second
	max429Wait       = 120 * time.Second
	politenessDelay  = 100 * time.Millisecond
	crawlJobTimeout  = 25 * time.Minute

	userAgent = "ecommerce-crawler-api/1.0 (+https://books.toscrape.com; study crawler)"

	catalogBaseURL      = "https://books.toscrape.com/catalogue/"
	catalogFirstPageURL = catalogBaseURL + "page-1.html"

	htmlClassProductPod     = "product_pod"
	htmlClassStarRating     = "star-rating"
	htmlClassPriceColor     = "price_color"
	htmlClassPaginationNext = "next"
)

var httpClient = &http.Client{
	Timeout: httpTimeout,
}

// Book representa um item do catálogo books.toscrape.com.
type Book struct {
	Title  string `json:"title"`
	Price  string `json:"price"`
	Rating string `json:"rating"`
}

// booksExport agrupa metadados da execução com a lista de livros (saída JSON legível).
type booksExport struct {
	Source    string `json:"source"`
	FetchedAt string `json:"fetched_at"`
	Count     int    `json:"count"`
	Books     []Book `json:"books"`
}

// sleepWithContext aguarda até d ou até o contexto ser cancelado.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// parseRetryAfterSeconds interpreta Retry-After em segundos (formato mais comum em APIs).
func parseRetryAfterSeconds(h http.Header) time.Duration {
	s := strings.TrimSpace(h.Get("Retry-After"))
	if s == "" {
		return 0
	}
	sec, err := strconv.Atoi(s)
	if err != nil || sec < 0 {
		return 0
	}
	d := time.Duration(sec) * time.Second
	if d > max429Wait {
		return max429Wait
	}
	return d
}

func backoffAfterFailure(attemptIndex int, hint time.Duration) time.Duration {
	if hint > 0 {
		return hint
	}
	d := initialBackoff * time.Duration(1<<uint(attemptIndex))
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

func snippetFromBody(body []byte) string {
	snippet := strings.TrimSpace(string(body))
	if len(snippet) > errSnippetLen {
		snippet = snippet[:errSnippetLen] + "…"
	}
	return snippet
}

func httpPageError(pageURL string, statusCode int, body []byte) error {
	return fmt.Errorf("GET %s: status %d, snippet: %s", pageURL, statusCode, snippetFromBody(body))
}

// doFetchOnce executa uma única tentativa GET.
func doFetchOnce(ctx context.Context, pageURL string) (parsed *html.Node, shouldRetry bool, retryAfterHint time.Duration, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, false, 0, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, true, 0, fmt.Errorf("GET %s: %w", pageURL, err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, true, 0, fmt.Errorf("read body %s: %w", pageURL, err)
	}

	if len(body) > maxBodyBytes {
		return nil, false, 0, fmt.Errorf("GET %s: body larger than %d bytes", pageURL, maxBodyBytes)
	}

	switch code := resp.StatusCode; {
	case code == http.StatusOK:
		root, parseErr := html.Parse(strings.NewReader(string(body)))
		if parseErr != nil {
			return nil, false, 0, fmt.Errorf("parse html %s: %w", pageURL, parseErr)
		}
		return root, false, 0, nil

	case code == http.StatusTooManyRequests:
		return nil, true, parseRetryAfterSeconds(resp.Header), httpPageError(pageURL, code, body)

	case code == http.StatusBadGateway || code == http.StatusServiceUnavailable || code == http.StatusGatewayTimeout:
		return nil, true, 0, httpPageError(pageURL, code, body)

	case code >= 500:
		return nil, true, 0, httpPageError(pageURL, code, body)

	default:
		return nil, false, 0, httpPageError(pageURL, code, body)
	}
}

func fetchPage(ctx context.Context, pageURL string) (*html.Node, error) {
	var lastErr error
	var lastHint time.Duration

	for attempt := 0; attempt < maxFetchAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if attempt > 0 {
			wait := backoffAfterFailure(attempt-1, lastHint)
			lastHint = 0
			if err := sleepWithContext(ctx, wait); err != nil {
				return nil, err
			}
		}

		doc, retry, hint, err := doFetchOnce(ctx, pageURL)
		if err == nil {
			return doc, nil
		}
		lastErr = err
		lastHint = hint
		if !retry {
			return nil, err
		}
	}

	return nil, fmt.Errorf("after %d attempts: %w", maxFetchAttempts, lastErr)
}

func parseBooks(doc *html.Node) []Book {
	var out []Book
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && hasClass(n, htmlClassProductPod) {
			out = append(out, extractBook(n))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

func extractBook(n *html.Node) Book {
	var title, price, rating string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "p":
				if hasClass(node, htmlClassStarRating) {
					rating = starRatingFromClassAttr(node)
				}
				if hasClass(node, htmlClassPriceColor) && node.FirstChild != nil {
					price = strings.TrimSpace(node.FirstChild.Data)
				}
			case "a":
				if title == "" {
					title = titleAttrFromAnchor(node)
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return Book{Title: title, Price: price, Rating: rating}
}

func starRatingFromClassAttr(n *html.Node) string {
	for _, a := range n.Attr {
		if a.Key != "class" {
			continue
		}
		parts := strings.Fields(a.Val)
		if len(parts) > 1 {
			return parts[1]
		}
	}
	return ""
}

func titleAttrFromAnchor(n *html.Node) string {
	for _, a := range n.Attr {
		if a.Key == "title" {
			return a.Val
		}
	}
	return ""
}

func getNextPage(doc *html.Node, currentURL string) (string, error) {
	var href string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "li" && hasClass(n, htmlClassPaginationNext) {
			href = firstHrefInElement(n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if href == "" {
		return "", nil
	}

	base, err := url.Parse(currentURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(href)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}

func firstHrefInElement(li *html.Node) string {
	for c := li.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "a" {
			for _, a := range c.Attr {
				if a.Key == "href" {
					return a.Val
				}
			}
		}
	}
	return ""
}

func atomicWriteFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		if rem := os.Remove(path); rem != nil && !os.IsNotExist(rem) {
			return fmt.Errorf("rename %s -> %s: %w (remove dest: %v)", tmp, path, err, rem)
		}
		if err2 := os.Rename(tmp, path); err2 != nil {
			return fmt.Errorf("rename %s -> %s: %w", tmp, path, err2)
		}
	}
	return nil
}

func writeBooksJSON(path string, books []Book) error {
	out := booksExport{
		Source:    catalogBaseURL,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		Count:     len(books),
		Books:     books,
	}

	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')

	return atomicWriteFile(path, payload)
}

func crawl(ctx context.Context) error {
	var all []Book
	cur := catalogFirstPageURL

	for cur != "" {
		if err := ctx.Err(); err != nil {
			return err
		}
		fmt.Println("Coletando:", cur)

		doc, err := fetchPage(ctx, cur)
		if err != nil {
			return fmt.Errorf("fetch %s: %w", cur, err)
		}

		all = append(all, parseBooks(doc)...)

		next, err := getNextPage(doc, cur)
		if err != nil {
			return err
		}
		if next == "" || next == cur {
			break
		}
		cur = next

		if err := sleepWithContext(ctx, politenessDelay); err != nil {
			return err
		}
	}

	if err := writeBooksJSON(defaultJSONPath, all); err != nil {
		return fmt.Errorf("escrever json: %w", err)
	}
	fmt.Printf("Exportados %d livros para %s\n", len(all), defaultJSONPath)

	for _, b := range all {
		fmt.Printf("Título: %s | Preço: %s | Avaliação: %s\n", b.Title, b.Price, b.Rating)
	}
	return nil
}

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

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), crawlJobTimeout)
	defer cancel()

	if err := crawl(ctx); err != nil {
		log.Fatal(err)
	}
}
