// Lenovo API — serviço HTTP que expõe notebooks Lenovo do site de testes webscraper.io.
package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"ecommerce-crawler-api/internal/lenovo/api"
	"ecommerce-crawler-api/internal/lenovo/service"
)

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}

	cacheTTL := 5 * time.Minute
	if s := os.Getenv("CACHE_TTL_SECONDS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			cacheTTL = time.Duration(n) * time.Second
		}
	}

	srv := &api.Server{
		Addr:     addr,
		CacheTTL: cacheTTL,
		Scraper:  &service.Scraper{},
	}

	base := "http://127.0.0.1" + addr
	if addr[0] != ':' {
		base = "http://" + addr
	}
	log.Printf("lenovo-api ouvindo em %s — o processo fica aqui até você encerrar (Ctrl+C).", addr)
	log.Printf("O crawler só roda quando alguém chama a API. Em outro terminal ou no navegador:")
	log.Printf("  curl -sS %s/health", base)
	log.Printf("  curl -sS %s/api/v1/lenovo-laptops  (primeira vez pode levar ~30–90s)", base)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
