package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"ecommerce-crawler-api/internal/lenovo/model"
	"ecommerce-crawler-api/internal/lenovo/service"
)

const defaultAddr = "127.0.0.1:8080"

// Response envelope JSON da API REST.
type Response struct {
	FetchedAt string               `json:"fetched_at"`
	Count     int                  `json:"count"`
	Products  []model.LenovoLaptop `json:"products"`
	Error     string               `json:"error,omitempty"`
}

type Server struct {
	Scraper  *service.Scraper
	Addr     string
	CacheTTL time.Duration
	mu       sync.Mutex
	cached   []model.LenovoLaptop
	cachedAt time.Time
}

func (s *Server) addr() string {
	if s.Addr != "" {
		return s.Addr
	}
	return defaultAddr
}

func (s *Server) getCached(now time.Time) ([]model.LenovoLaptop, time.Time, bool) {
	if s.CacheTTL <= 0 {
		return nil, time.Time{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cachedAt.IsZero() {
		return nil, time.Time{}, false
	}
	if now.Sub(s.cachedAt) > s.CacheTTL {
		return nil, time.Time{}, false
	}
	cp := append([]model.LenovoLaptop(nil), s.cached...)
	return cp, s.cachedAt, true
}

func (s *Server) setCached(items []model.LenovoLaptop, now time.Time) {
	if s.CacheTTL <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cached = append([]model.LenovoLaptop(nil), items...)
	s.cachedAt = now
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func (s *Server) handleLenovo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx := r.Context()
	force := r.URL.Query().Get("refresh") == "1" || r.URL.Query().Get("refresh") == "true"
	now := time.Now().UTC()

	if !force {
		if items, at, ok := s.getCached(now); ok {
			log.Printf("lenovo-laptops: cache hit (%d itens, fetched_at=%s)", len(items), at.UTC().Format(time.RFC3339))
			writeJSON(w, http.StatusOK, Response{
				FetchedAt: at.UTC().Format(time.RFC3339),
				Count:     len(items),
				Products:  items,
			})
			return
		}
	}

	log.Printf("lenovo-laptops: iniciando coleta no webscraper.io (pode demorar)...")
	items, err := s.Scraper.FetchLenovoLaptopsSorted(ctx)
	if err != nil {
		log.Printf("lenovo-laptops: erro na coleta: %v", err)
		writeJSON(w, http.StatusBadGateway, Response{
			FetchedAt: now.Format(time.RFC3339),
			Error:     err.Error(),
		})
		return
	}
	s.setCached(items, now)
	log.Printf("lenovo-laptops: coleta concluída (%d produtos Lenovo)", len(items))
	writeJSON(w, http.StatusOK, Response{
		FetchedAt: now.Format(time.RFC3339),
		Count:     len(items),
		Products:  items,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(`{"status":"ok"}` + "\n"))
}

// ListenAndServe registra rotas e bloqueia até erro do servidor HTTP.
func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/v1/lenovo-laptops", s.handleLenovo)

	srv := &http.Server{
		Addr:              s.addr(),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.ListenAndServe()
}
