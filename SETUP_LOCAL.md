# Setup local

O setup usa Docker Compose com um load balancer Nginx e duas instâncias da API.
Cada API carrega os recursos em `resources/`, vetoriza o payload, busca os cinco
vizinhos mais próximos e aplica o score de fraude documentado.

## Subir o ambiente

```bash
docker compose up --build
```

O endpoint público fica em `http://localhost:9999`.

Os caminhos dos recursos podem ser substituídos por variáveis de ambiente:
`REFERENCES_PATH`, `MCC_RISK_PATH` e `NORMALIZATION_PATH`. O endereço padrão da
API continua sendo `:9999`.

## Testar prontidão

```bash
curl -i http://localhost:9999/ready
```

## Testar o balanceamento

O header `X-API-Instance` permite confirmar que as duas instâncias estão
recebendo requisições:

```bash
for i in $(seq 1 6); do
  curl -s -D - -o /dev/null -X POST http://localhost:9999/fraud-score
done
```

Para enviar uma requisição real, use um payload no formato descrito em
`docs/br/API.md`. A resposta contém somente `approved` e `fraud_score`.

## Medir localmente

Com o Compose em execução, o script padrão envia requisições concorrentes e
calcula throughput, p50, p95 e p99:

```bash
python3 scripts/load_test.py --requests 200 --concurrency 16
```

O benchmark interno da busca usa o dataset oficial e deve ser executado com
uma única iteração, porque o carregamento do dataset acontece uma vez por API:

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.24 \
  go test ./internal/search -run '^$' \
  -bench '^BenchmarkBruteForceTopKOfficial$' -benchtime=1x
```

## Parar e limpar

```bash
docker compose down
```

Os limites declarados somam `0.60` CPU e `290M` de memória. A rede é `bridge` e
os serviços estão configurados para `linux/amd64`.
