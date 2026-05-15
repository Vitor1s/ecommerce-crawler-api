# ecommerce-crawler-api

Monorepo em **Go** para um teste técnico em duas partes: **crawler HTTP** (catálogo de livros) e **API REST** (notebooks Lenovo no site de testes do Web Scraper). Não há automação de navegador — apenas `net/http` e parsing de HTML.

**Por que Go:** decidi usar Go porque gosto da linguagem e acho o funcionamento dela **interessante para esse tipo de tarefa** — binário simples de distribuir, biblioteca padrão sólida para HTTP/JSON e um modelo de concorrência claro quando a coleta precisar evoluir sem stack pesada para crawlers e APIs pequenas. **Node.js** também seria uma escolha muito bacana para o mesmo desafio (Fetch/Axios, `cheerio`/`node-html-parser`, Express/Fastify): o enunciado permitia as duas stacks; foquei em Go por preferência pessoal neste repo.

## Estrutura do repositório

```
ecommerce-crawler-api/
├── README.md                 ← este guia (visão geral)
├── go.mod / go.sum
├── cmd/
│   ├── books-crawler/        Parte 2 — crawler books.toscrape.com
│   │   ├── main.go
│   │   └── README.md         ← análise, decisões técnicas e como rodar
│   └── lenovo-api/           Parte 1 — API Lenovo + webscraper.io
│       ├── main.go
│       └── README.md         ← endpoints, env, troubleshooting
└── internal/lenovo/          lógica da API (scrape, service, model)
```

Cada parte tem o **próprio README** com o detalhe (problemas corrigidos, trade-offs, exemplos de `curl`, etc.).

## Requisitos

- [Go](https://go.dev/dl/) compatível com o `go.mod` do projeto (1.22+; o módulo declara 1.25).
- Acesso à internet nas execuções (sites externos).

Na raiz do clone:

```bash
go mod download
```

## Parte 1 — API Lenovo (desafio prático)

Servidor HTTP que coleta laptops **Lenovo**, ordena do mais barato ao mais caro e devolve **JSON**.

```bash
go run ./cmd/lenovo-api
```

Em outro terminal (com o servidor ainda rodando):

```bash
curl -sS http://127.0.0.1:8080/health
curl -sS http://127.0.0.1:8080/api/v1/lenovo-laptops
```

O mesmo fluxo pode ser feito com **Postman**, **Insomnia**, **Bruno**, **Hoppscotch**, **Thunder Client** (VS Code) ou qualquer cliente HTTP — `GET` na mesma base URL e caminhos acima.

**Documentação completa:** [`cmd/lenovo-api/README.md`](cmd/lenovo-api/README.md)

---

## Parte 2 — Crawler books.toscrape.com (revisão de código)

CLI que percorre o catálogo, aplica retries/politeness e grava **`books.json`** no diretório de trabalho atual (rode a partir da raiz do repositório se quiser o arquivo na raiz).

```bash
go run ./cmd/books-crawler
```

**Documentação completa (análise + justificativas):** [`cmd/books-crawler/README.md`](cmd/books-crawler/README.md)

---

## Build rápido dos dois binários

```bash
go build -o books-crawler ./cmd/books-crawler
go build -o lenovo-api     ./cmd/lenovo-api
```

