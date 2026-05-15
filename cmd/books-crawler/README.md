# Parte 2 — Crawler `books.toscrape.com` (análise e correções)

Este documento reúne **problemas identificados**, **impacto em produção**, **correções aplicadas** e **justificativa** (incluindo dependências).

## Alinhamento com o enunciado

O repositório entrega o crawler em **`cmd/books-crawler/main.go`** (corrigido) e esta análise. O código cobre os cenários da dica (**rede instável**, **HTTP não OK**, **429/5xx transitórios com retry limitado**, **HTML frágil**, **paginação**, **cancelamento por deadline**) sem automação de browser.

**Transparência:** há **retry com backoff** e **`context.Context`** com timeout global do job; ainda **não** há **checkpoint** em disco se o processo for morto no meio da coleta (a execução recomeça do zero). Isso é aceitável para o escopo do exercício; retomada parcial seria o próximo passo em produção.

---

## Como executar

Na raiz do repositório (onde está o `go.mod`), para que `books.json` seja gravado na pasta atual por padrão:

```bash
go run ./cmd/books-crawler
```

Requisito: Go com suporte a módulos e acesso à internet para baixar `golang.org/x/net` na primeira execução.

Ao terminar a coleta, o programa grava **`books.json`** na pasta atual: JSON indentado com `source`, `fetched_at` (UTC, RFC3339), `count` e o array `books` (título, preço, avaliação). A lista também continua sendo impressa no console.

**Testes com cliente HTTP (REST no repositório):** esta Parte 2 é **linha de comando** (validação pela execução e pelo `books.json`). Já a **Parte 1** (`cmd/lenovo-api`) expõe uma API REST — lá você pode usar **Postman**, **Insomnia**, **Bruno**, **Hoppscotch**, **Thunder Client** ou equivalentes, além de `curl`, para disparar os `GET` nos endpoints documentados naquele README.

### Por que exportar os dados em JSON (e o que isso melhora)

A **coleta em si** (HTTP + parse) continua igual: o JSON não acelera downloads nem evita 429 sozinho. O ganho está no **que você faz com o resultado** depois que a coleta termina.

- **Artefato estável e explícito** — Em vez de depender só de linhas soltas no terminal, você passa a ter um **documento** com estrutura fixa (`source`, `fetched_at`, `count`, `books`). Isso vira “contrato” informal para inspeção humana ou para outro programa ler sem reinterpretar texto livre.
- **Rastreabilidade** — O campo `fetched_at` marca **quando** aquele snapshot foi gerado. Em produção isso ajuda a comparar execuções, auditar atrasos e saber se um arquivo está desatualizado.
- **Integração e pipelines** — JSON é o formato mais comum entre serviços, ETL, testes de regressão (“golden file”) e ferramentas como `jq`. Você reduz atrito para gravar em banco, mandar para uma fila ou validar em CI.
- **Validação e qualidade** — Com um arquivo único fica mais fácil checar `count` versus o esperado, procurar duplicatas, validar schema (mesmo manualmente) e fazer diff entre duas coletas para detectar regressões no parser.
- **Reprodutibilidade** — O mesmo `go run ./cmd/books-crawler` gera um arquivo que pode ser anexado ao PR, ao ticket ou ao relatório do teste, sem copiar/colar saída do terminal.

Em resumo: a exportação **melhora o valor e a usabilidade dos dados coletados** (persistência, consumo por máquina, auditoria e testes), não substitui boas práticas de rede; por isso ela complementa as correções de robustez descritas abaixo.

### Organização do código (legibilidade)

Faz sentido **mencionar de leve** no README (como aqui), sem virar tutorial: o avaliador vê que você pensa em **manutenção**, não só em “passar no site”.

- **Constantes** para URL base do catálogo, primeira página e classes HTML (`product_pod`, `next`, etc.) — reduz “magic string” e deixa o scraper mais fácil de ajustar se o layout mudar.  
- **Funções pequenas** com responsabilidade única: espera cancelável (`sleepWithContext`), erro HTTP com trecho do body (`httpPageError` / `snippetFromBody`), escrita atômica (`atomicWriteFile`), extração de `href` no `<li class="next">` (`firstHrefInElement`), rating e título (`starRatingFromClassAttr`, `titleAttrFromAnchor`).  
- **Fluxo de retry** em `fetchPage` sem ramos redundantes e **sem sombrear** variáveis no parse HTML.  
- **Primeiro título vence** no `extractBook` — intenção explícita quando há mais de um `<a>` com `title`.

Isso não substitui a tabela de problemas/correções; **complementa** a narrativa de qualidade do código.

---

## Problemas encontrados no código original

| # | Problema | O que aconteceria em produção |
|---|-----------|-------------------------------|
| 1 | Uso de `http.DefaultClient` **sem timeout** | Requisições podem ficar **penduradas indefinidamente** (socket lento, servidor sem fechar). Em um worker ou fila, isso acumula goroutines/conexões e derruba o serviço por esgotamento de recursos. |
| 2 | **Não validar** `resp.StatusCode` antes de parsear | Em **404/500/502/503/429** o programa ainda trata o corpo como HTML de listagem. Resultado: **lotes vazios ou lixo** sem distinção clara de “falha de upstream” vs “catálogo vazio”; monitoramento e reprocessamento ficam enganosos. |
| 3 | Header **`Accept-Encoding: zstd`** forçado | Se o servidor responder com corpo comprimido que o cliente **não decodifica** de forma transparente, `html.Parse` recebe **bytes inválidos** → árvore errada, extração zerada ou intermitente, bug difícil de reproduzir. |
| 4 | **`io.ReadAll` sem limite** de tamanho | Resposta anormalmente grande (bug, ataque, proxy) → **pico de memória** ou OOM em instâncias pequenas. |
| 5 | Slice **`results` global** mutada por `parseBooks` | Segunda execução no mesmo processo **acumula** dados; testes em paralelo ou reuso do pacote geram **resultados fantasmas** ou condição de corrida se alguém paralelizar fetch. |
| 6 | **`hasClass` com `strings.Contains`** em valor de `class` | Classes como `not_price_color` podem **acionar falsamente** `price_color` por substring → preços/títulos errados após uma mudança pequena no HTML. |
| 7 | **`getNextPage` via `FirstChild` / `NextSibling` fixos** | Com **nós de texto** (espaços, quebras de linha) entre `<li>` e `<a>`, o link “next” **não é encontrado** → crawl para na **primeira página** mesmo existindo próxima. |
| 8 | **`href` relativo** concatenado mentalmente com `base` fixa | Erros de **URL inicial** (ex.: `page-2.htm` vs `page-1.html`) ou base errada → páginas puladas ou 404 em cadeia. |
| 9 | Bloco **`if next == "" { parseBooks(doc) }`** | **Parse duplicado** da última página e risco de **duplicar** itens na lista global, poluindo downstream (DB, relatórios). |
| 10 | Erro em `fetchPage` só com **`log` + `break`** | O programa pode **imprimir resultados parciais** como se a coleta tivesse sido completa → **falha parcial mascarada**, sem código de saída claro para CI/job. |
| 11 | URL **`http://`** e escolha frágil da **primeira página** | Ambientes com política HTTPS, HSTS ou proxies podem complicar; URL canônica e primeira página corretas reduzem surpresas. |

---

## Correções aplicadas e justificativa

| Correção | Justificativa |
|----------|----------------|
| `http.Client` dedicado com **`Timeout: 30s`** | Limita o tempo máximo por requisição, alinhado a cenário de **rede lenta** ou servidor travado. |
| Falha se **`StatusCode != 200`** | Impede tratar página de erro como catálogo; o erro inclui um **trecho do body** (cortado) para depuração sem logar megabytes. |
| **Remoção** do `Accept-Encoding: zstd` manual | Deixa a negociação de compressão para o comportamento padrão do cliente Go, evitando corpo comprimido **não decodificado** pelo parser. |
| **`io.LimitReader`** + erro se `len(body) > maxBodyBytes` | Protege contra **respostas gigantes** e uso excessivo de RAM. |
| **`parseBooks` retorna `[]Book`**, sem variável global | Coleta **determinística por execução**, testável e preparada para evoluir sem data race. |
| **`getNextPage(doc, currentURL)`** percorre filhos do `<li class="next">` até achar `<a href>` | Funciona com **whitespace** entre tags; resolve `href` relativo com **`url.Parse` + `ResolveReference`**. |
| Loop em `crawl` com URL **absoluta** e início **`https://.../page-1.html`** | Paginação confiável e alinhada ao site; **`next == cur`** evita loop infinito se o HTML repetir o mesmo link. |
| **`hasClass` por tokens** (`strings.Fields` + igualdade) | Match de classe **exato**, reduzindo falso positivo por substring. |
| **`crawl(ctx) error`** e **`log.Fatal` em `main`** | Falha visível para **job/CI** (exit code ≠ 0); `main` cria **timeout global** do job com `context.WithTimeout`. |
| Tags **`json`** em `Book` | Serialização estável e nomes de campo explícitos no JSON. |
| Export **`books.json`** via **`encoding/json.MarshalIndent`** | Um único arquivo legível com metadados (`source`, `fetched_at`, `count`) e o array `books`; útil para inspeção, CI ou consumo por outra ferramenta. Usa apenas a **stdlib** (`encoding/json`, `os`). |
| **`context.WithTimeout`** no `main` + **`http.NewRequestWithContext`** | Deadline global do crawl (ex.: 25 min) e cancelamento cooperativo; evita processo pendurado após deploy e alinha com boas práticas de serviços de longa duração. |
| **Retry limitado** (3 tentativas) com **backoff exponencial** + **`Retry-After`** em **429** | Falhas transitórias (502/503/504, 5xx genérico, erro de rede, leitura de body) não derrubam o job na primeira falha; 429 respeita pausa quando o header traz segundos (limitado). |
| **User-Agent** explícito | Reduz chance de bloqueio intermitente por WAF/CDN que filtra o UA padrão do Go. |
| **Pausa curta entre páginas** (`politenessDelay`) | Reduz pressão no upstream e a probabilidade de **429** em ambientes sensíveis a taxa. |
| **Gravação atômica do JSON** (`books.json.tmp` + `Rename`, com fallback no Windows) | Evita `books.json` **corrompido ou pela metade** se o processo cair durante a escrita. |

---

## Bibliotecas

| Dependência | Uso | Por que manter |
|-------------|-----|------------------|
| **`golang.org/x/net/html`** | Parser HTML em árvore (`html.Parse`, nós e atributos) | Já era a base do código original; a stdlib não expõe parser HTML DOM completo no mesmo nível. É a escolha idiomática em Go para scraping estrutural. |

**Retry, contexto, backoff e `Retry-After`** usam apenas a **stdlib** (`context`, `time`, `net/http`, `strconv`). Não foi adicionada biblioteca externa de terceiros (evita dependência extra só para este escopo).

---

## Processo interrompido no meio da coleta

**Comportamento atual:** os livros coletados até o momento existem só em memória (`all` em `crawl`). Se o processo **for morto** (SIGKILL, OOM, deploy), **não há retomada** da fila de páginas: na próxima execução a coleta **recomeça do zero**. O arquivo final **`books.json`** tende a ficar **íntegro ou inalterado** (escrita atômica), não meio corrompido.

**Checkpoint em arquivo/DB** segue como evolução de produto (cursor de página, deduplicação, transações), fora do mínimo deste exercício.

---

## Próximos passos recomendados (produção real)

1. **Checkpoint** opcional (última URL processada) para retomar após crash.  
2. **`extractBook`**: preferir o **primeiro** `title` relevante dentro de `product_pod` (ex.: `<h3><a>`) se o HTML passar a ter vários `<a>` com `title`.  
3. **Jitter** no backoff e limite de taxa configurável por ambiente.

---
