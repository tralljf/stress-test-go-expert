# Stress Test CLI (Go)

CLI em Go para testes de carga em serviços web. Você informa a URL, o número total de requisições e a quantidade de chamadas simultâneas; ao final, o programa mostra um relatório com tempo total, total de requests, quantidade de HTTP 200, distribuição dos demais status e erros sem resposta.

Desafio da Pós Go Expert (Full Cycle). Usa só a biblioteca padrão do Go (Go 1.24).

## Executando com Docker (forma recomendada)

### 1. Buildar a imagem

```bash
docker build -t stress-test .
```

O build é multi-stage: compila em `golang:1.24-alpine` e gera uma imagem final `scratch` (cerca de 6 MB) com o binário estático e os certificados CA, então URLs HTTPS também funcionam. O `ENTRYPOINT` aponta para o binário, por isso as flags vão direto depois do nome da imagem.

### 2. Rodar o teste

```bash
docker run --rm stress-test --url=http://google.com --requests=1000 --concurrency=10
```

Outros exemplos:

```bash
# status variados (o httpbin sorteia um dos códigos da lista)
docker run --rm stress-test --url=https://httpbin.org/status/200,404,500,503 --requests=40 --concurrency=8

# timeout menor por requisição
docker run --rm stress-test --url=https://example.com --requests=100 --concurrency=20 --timeout=2s

# serviço rodando na sua máquina (Docker Desktop)
docker run --rm stress-test --url=http://host.docker.internal:8080 --requests=500 --concurrency=50
```

## Executando sem Docker

```bash
go run . --url=http://google.com --requests=100 --concurrency=10
# ou
go build -o stress-test . && ./stress-test --url=http://google.com --requests=100 --concurrency=10
```

## Flags

| Flag | Obrigatória | Padrão | Descrição |
|------|-------------|--------|-----------|
| `--url` | sim | | URL do serviço a ser testado. Precisa ser `http` ou `https` e ter host. |
| `--requests` | sim | | Número total de requisições, `>= 1`. |
| `--concurrency` | não | `1` | Número de chamadas simultâneas, `>= 1`. Se for maior que `--requests`, é limitada a `--requests`. |
| `--timeout` | não | `10s` | Timeout de cada requisição (formato Go: `500ms`, `5s`, `1m`). |
| `--max-redirects` | não | `10` | Máximo de redirects seguidos por requisição. `0` desliga o seguimento. |

As flags aceitam tanto `--flag=valor` quanto `--flag valor`. Com entrada inválida, o programa mostra o erro e sai com código `2`. `--help` lista as flags.

## Exemplo de saída

Saída real de `docker run --rm stress-test --url=http://google.com --requests=1000 --concurrency=10`:

```
Running 1000 requests against http://google.com with concurrency 10 (timeout 10s, max redirects 10)...
==================================================
 STRESS TEST REPORT
==================================================
URL:                      http://google.com
Concurrency:              10
Total time:               1m15.604s
Total requests:           1000
Requests/second:          13.23
HTTP 200 (OK):            891

Other HTTP status codes:
  302 Found                          109

Errors (no HTTP response): 0
==================================================
```

Os 302 são o Google limitando a taxa: depois de algumas centenas de requisições ele passa a mandar o cliente para a página `/sorry/`, que redireciona em loop. Veja a seção sobre redirects.

Exemplo com timeouts (endereço que não responde):

```
Errors (no HTTP response): 6
  timeout                            6
    e.g.: Get "http://10.255.255.1": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
```

A soma de `HTTP 200` + outros status + erros é sempre igual a `Total requests`.

## Como funciona

- **Número exato de requests**: `concurrency` workers (goroutines) disputam um contador atômico (`atomic.Int64`). Cada worker incrementa o contador e só faz a requisição se o valor obtido for `<= requests`. Assim são feitas exatamente `requests` requisições, sem condição de corrida e sem precisar de canal de jobs.
- **Agregação sem lock**: cada worker grava os resultados no seu próprio relatório parcial. Os parciais são somados depois do `WaitGroup.Wait()`.
- **Cliente HTTP**: um único `http.Client` compartilhado, com keep-alive, `MaxIdleConnsPerHost = concurrency` (para cada worker reaproveitar a própria conexão) e timeout por requisição (`--timeout`). O corpo de toda resposta é lido até o fim e descartado (`io.Copy(io.Discard, ...)`) antes do `Close`, o que permite reaproveitar a conexão.
- **Redirects**: o cliente segue redirects (comportamento padrão do Go) até `--max-redirects` (padrão 10, o mesmo limite do Go), e o status contabilizado é o da **resposta final**. Por isso `http://google.com` (301 para `www.google.com`) aparece como 200. Uma diferença em relação ao padrão do Go: quando o limite é atingido, a última resposta 3xx é contabilizada como status HTTP em vez de virar erro, porque o servidor respondeu. Com `--max-redirects=0` nenhum redirect é seguido e o 301/302 original é contabilizado.
- **Erros sem resposta**: timeouts, conexão recusada, DNS etc. não têm status HTTP. Eles entram no total e aparecem na seção `Errors`, agrupados por categoria (`timeout`, `connection refused`, `connection reset`, `dns error`, `canceled`, `other error`) e com uma mensagem de exemplo de cada categoria.
- **Ctrl+C**: cancela as requisições em andamento. As restantes falham na hora como `canceled` e o relatório é exibido mesmo assim.

## Estrutura

```
.
├── main.go                      # entrada do CLI: parse das flags, execução e relatório
├── internal/config              # parse e validação das flags (+ testes)
├── internal/loadtest            # cliente HTTP, workers, agregação e relatório (+ testes)
├── Dockerfile                   # build multi-stage, imagem final scratch + certificados CA
└── go.mod
```

## Testes

```bash
go vet ./...
go test -race ./...
```

Os testes sobem um `httptest.Server` e cobrem:

- contagem exata de requisições sob concorrência (o servidor conta os hits com contador atômico), em várias combinações de `requests`/`concurrency`, incluindo `concurrency > requests`;
- distribuição de status: o servidor alterna 200/404/500 e o relatório tem que bater exatamente;
- redirect seguido (status final contado) e limite de redirects (3xx contabilizado);
- timeouts e conexão recusada contabilizados como erros;
- validação das flags (URL ausente ou inválida, requests e concurrency `< 1`, valores não numéricos, flag desconhecida, limite de concurrency);
- formato do relatório.
