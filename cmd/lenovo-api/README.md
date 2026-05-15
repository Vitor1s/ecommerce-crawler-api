# Lenovo laptops API 

Serviço em **Go** que coleta notebooks **Lenovo** em  
[Computers / Laptops — static e-commerce](https://webscraper.io/test-sites/e-commerce/static/computers/laptops),  
enriquece cada item com a **página do produto**, ordena do **mais barato ao mais caro** e expõe o resultado em **JSON** via **REST**.

## Requisitos

- Go **1.22+** (o `go.mod` do repositório usa 1.25; use a mesma ou compatível).
- Rede para acessar `https://webscraper.io`.

## Como rodar

Na raiz do repositório `ecommerce-crawler-api`:

```bash
go run ./cmd/lenovo-api
```

Por padrão o servidor sobe em **`127.0.0.1:8080`** e **fica em execução** até você interromper (`Ctrl+C`). Ele **não** imprime a lista sozinho: a coleta só roda quando algo faz `GET` em `/api/v1/lenovo-laptops` (use outro terminal ou o navegador).

Variáveis de ambiente opcionais:

| Variável | Descrição | Padrão |
|----------|-----------|--------|
| `LISTEN_ADDR` | Endereço do bind (`127.0.0.1:8080`, `:8080` para todas as interfaces, etc.) | `127.0.0.1:8080` |
| `CACHE_TTL_SECONDS` | TTL do cache em memória (0 = sem cache) | `300` (5 min) |

### Endpoints

- `GET /health` — verificação simples (`{"status":"ok"}`).
- `GET /api/v1/lenovo-laptops` — lista JSON de notebooks Lenovo ordenados por preço.
- `GET /api/v1/lenovo-laptops?refresh=true` — ignora cache e força nova coleta.

Exemplo:

```bash
curl -s http://127.0.0.1:8080/api/v1/lenovo-laptops | head
```

### Testar com Postman (ou outro cliente HTTP)

Os mesmos endpoints podem ser exercidos com **Postman**, **Insomnia**, **Bruno**, **Hoppscotch**, extensões como **Thunder Client** (VS Code) ou qualquer ferramenta que envie requisições **HTTP**. Use método **GET**, sem corpo; base URL típica: `http://127.0.0.1:8080` (ou a porta definida em `LISTEN_ADDR`). Exemplos de caminho: `/health`, `/api/v1/lenovo-laptops`, `/api/v1/lenovo-laptops?refresh=true`.

### `curl: (7) Failed to connect` / “Could not connect to server”

Quase sempre significa **nenhum processo escutando nessa porta** (ou porta errada).

1. **Deixe o servidor rodando** num terminal: `go run ./cmd/lenovo-api` — não feche nem rode o `curl` antes disso.
2. Use **o mesmo tipo de ambiente** para servidor e `curl`: se o `go run` está no **PowerShell do Windows**, o `curl` também deve ser no Windows; se está no **WSL (Ubuntu, etc.)**, rode o `curl` dentro do WSL. Misturar (servidor no WSL e `curl` só no Windows sem port forward correto, ou o contrário) costuma falhar.
3. No Windows, confira se algo está em escuta: `netstat -ano | findstr ":8080"` — deve aparecer `LISTENING` enquanto o `lenovo-api` estiver ativo.
4. Se a porta 8080 estiver ocupada por outro programa, use outra, por exemplo:  
   `$env:LISTEN_ADDR="127.0.0.1:3000"; go run ./cmd/lenovo-api` e depois `curl http://127.0.0.1:3000/api/v1/lenovo-laptops`.

## O que o scraper faz

1. Percorre **todas** as páginas da listagem de laptops (link “next” da paginação).
2. Em cada card (`div.card.thumbnail`), lê nome, preço da listagem, descrição, `data-rating`, `reviewCount` e link relativo.
3. Mantém apenas produtos cuja marca **Lenovo** aparece no **nome ou na descrição** do card (case-insensitive). Depois da página do produto, **valida de novo** com nome+descrição da PDP para descartar inconsistências do site de teste (ex.: card “Lenovo” linkando PDP de outro modelo).
4. Linhas como **ThinkPad** só entram se a palavra **“Lenovo”** aparecer no nome ou na descrição (ex.: “Lenovo ThinkPad …”).
5. Para cada URL única, faz `GET` na **página do produto** e extrai preço canônico (pode diferir do card), descrição, imagem, reviews, estrelas (contagem de `ws-icon-star` ou `data-rating`) e opções de **HDD** (botões `.swatch`).
6. Ordena por `price_usd` crescente.

## Estrutura de pastas pra manter uma pratica que gosto de manter em projetos adequada e simples de entender.

```
cmd/lenovo-api/          # ponto de entrada (main)
internal/lenovo/
  model/                 # structs de domínio / JSON
  scrape/                # HTTP + parse HTML (sem browser headless)
  service/               # orquestração (listagem → filtro → detalhe → ordenação)
  api/                   # servidor HTTP, cache, handlers
```

`internal/` evita import acidental por outros módulos; `cmd/` separa binários.

## Decisões técnicas

- **Somente HTTP + parser HTML** (`net/http`, `golang.org/x/net/html`), no mesmo espírito do crawler `cmd/books-crawler`: timeouts, limite de corpo, retries com backoff e tratamento de 429/5xx.
- **User-Agent** explícito e **delay** curto entre requisições (~100 ms) para ser educado com o site de testes.
- **Página de produto**: o enunciado pede “todos os campos disponíveis na página”; a listagem não traz variantes de HDD nem preço “final” da PDP — por isso o segundo fetch por produto.
- **Cache em memória**: a primeira chamada dispara dezenas de `GET`s; o cache reduz carga em desenvolvimento e demos. `?refresh=true` força atualização.
- **Ordenação**: usa `price_usd` numérico parseado de strings `$…` para ordenação estável.
- **Dupla checagem “Lenovo”**: após parsear a PDP, o serviço descarta itens cujo nome+descrição na página do produto não contêm “Lenovo”, evitando ruído do catálogo de testes.

## Visão futura: requisição personalizada por perfil de notebook

Hoje o endpoint devolve **todos** os Lenovo já ordenados por preço. Um passo natural de produto (e de maturidade de API) é expor uma **requisição parametrizada** — por exemplo `GET /api/v1/lenovo-laptops?min_price_usd=300&max_price_usd=800&min_reviews=5&min_ram_gb=8` ou um `POST /api/v1/lenovo-laptops/search` com um JSON de critérios — em que o cliente descreve o **perfil desejado** (faixa de preço, RAM mínima, tamanho de tela, presença de SSD, linha ThinkPad vs IdeaPad, nota mínima, etc.) e o servidor **só serializa o que encaixa**, ainda ordenado por preço ou por outro score.

Do ponto de vista de implementação, isso encaixa bem com o que já existe: os dados da PDP e da descrição podem ser **normalizados** (parse de “4GB”, “15.6\"”, “SSD”) em campos estruturados no modelo; o filtro vira uma camada pura sobre uma **lista materializada** (memória, snapshot em Redis, ou tabela após ETL). Em um horizonte mais longo, dá para combinar isso com **busca semântica** ou ranking por relevância quando o usuário manda texto livre (“leve para viagem, até 14 polegadas”) — sempre com limites claros de custo e de carga no site de origem. Nada disso foi exigido no escopo atual do teste; fica registrado como **evolução consciente** da API, não como promessa de roadmap.

## Algo a mais que pensei no meio do caminho (JA3 / fingerprint de navegador)

Aqui usei o cliente HTTP padrão da biblioteca padrão. Para aproximar **TLS/JA3/HTTP2** de um navegador real em Go, o caminho usual é um cliente baseado em **utls** / **fhttp** (por exemplo ecossistemas como `tls-client` / `bogdanfinn`). Isso adiciona dependências e superfície de manutenção; documentamos aqui como evolução sem acoplar ao binário atual.

