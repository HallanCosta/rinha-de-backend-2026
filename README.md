# Rinha de Backend 2026 – Fraud Detection

![cover](./misc/cover.png)

[Português](#português) · [English](#english)

### Edição encerrada! Resultados oficiais em [rinhadebackend.com.br](https://rinhadebackend.com.br/) · Edition closed! Official results at [rinhadebackend.com.br](https://rinhadebackend.com.br/)

---

## Português

A **Rinha de Backend** é uma competição amistosa em que você constrói um backend sob restrições de CPU, memória, e arquitetura. Cada edição traz um tema diferente – e o desta vez é **detecção de fraudes usando busca vetorial**.

**Documentação completa do desafio:** [**docs/br/README.md**](./docs/br/README.md)

### Como a detecção funciona

Para cada requisição, transformamos a payload em um vetor numérico de 14
dimensões. Não é um embedding de IA: são cálculos determinísticos baseados nos
campos da transação.

```text
payload JSON
   ↓
vetorização matemática
   ↓
[14 números]
   ↓
comparação com o dataset
   ↓
5 vizinhos mais próximos
   ↓
fraud_score e approved
```

O vetor novo é comparado com o dataset de referências. Quanto menor a distância
euclidiana, mais parecidas são as transações. O resultado usa a regra:

```text
fraud_score = quantidade de vizinhos fraud / 5
approved = fraud_score < 0.6
```

### Case de sucesso: transação aprovada

Uma compra de baixo valor, próxima de casa e em um comerciante conhecido:

```bash
curl -i -X POST http://localhost:9999/fraud-score \
  -H 'Content-Type: application/json' \
  --data-raw '{
    "id": "tx-approved-example",
    "transaction": {
      "amount": 41.12,
      "installments": 2,
      "requested_at": "2026-03-11T18:45:53Z"
    },
    "customer": {
      "avg_amount": 82.24,
      "tx_count_24h": 3,
      "known_merchants": ["MERC-003", "MERC-016"]
    },
    "merchant": {
      "id": "MERC-016",
      "mcc": "5411",
      "avg_amount": 60.25
    },
    "terminal": {
      "is_online": false,
      "card_present": true,
      "km_from_home": 29.23
    },
    "last_transaction": null
  }'
```

Vetor gerado:

```text
[0.0041, 0.1667, 0.0500, 0.7826, 0.3333, -1, -1,
 0.0292, 0.1500, 0, 1, 0, 0.1500, 0.0060]
```

Os cinco vizinhos mais próximos são legítimos:

```text
0.0340 legit   0.0488 legit   0.0509 legit
0.0591 legit   0.0592 legit
```

```json
{
  "approved": true,
  "fraud_score": 0.0
}
```

### Case de falha: transação negada

Aqui temos uma compra de valor alto, distante de casa e em um comerciante
desconhecido:

```bash
curl -i -X POST http://localhost:9999/fraud-score \
  -H 'Content-Type: application/json' \
  --data-raw '{
    "id": "tx-denied-example",
    "transaction": {
      "amount": 9505.97,
      "installments": 10,
      "requested_at": "2026-03-14T05:15:12Z"
    },
    "customer": {
      "avg_amount": 81.28,
      "tx_count_24h": 20,
      "known_merchants": ["MERC-008", "MERC-007", "MERC-005"]
    },
    "merchant": {
      "id": "MERC-068",
      "mcc": "7802",
      "avg_amount": 54.86
    },
    "terminal": {
      "is_online": false,
      "card_present": true,
      "km_from_home": 952.27
    },
    "last_transaction": null
  }'
```

Vetor gerado:

```text
[0.9506, 0.8333, 1.0000, 0.2174, 0.8333, -1, -1,
 0.9523, 1.0000, 0, 1, 1, 0.7500, 0.0055]
```

Os cinco vizinhos mais próximos são fraudulentos:

```text
0.2315 fraud   0.2384 fraud   0.2552 fraud
0.2667 fraud   0.2785 fraud
```

```json
{
  "approved": false,
  "fraud_score": 1.0
}
```

### Como executar localmente

Suba o Nginx e as duas instâncias da API:

```bash
docker compose up --build -d
```

Verifique a prontidão. Durante os primeiros segundos o dataset ainda pode estar
sendo carregado:

```bash
curl -i http://localhost:9999/ready
```

Quando estiver pronto, a resposta será `HTTP 200` com `ready`.

#### Testes, benchmark e carga

Como o Go é executado dentro do Docker neste projeto:

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.24 go test ./...
docker run --rm -v "$PWD":/src -w /src golang:1.24 go vet ./...
```

Benchmark da busca com o dataset oficial:

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.24 \
  go test ./internal/search -run '^$' \
  -bench '^BenchmarkBruteForceTopKOfficial$' -benchtime=1x
```

Teste de carga com p50, p95 e p99:

```bash
python3 scripts/load_test.py --requests 200 --concurrency 16
```

Logs e memória dos containers:

```bash
docker compose logs -f api-1 api-2 load-balancer
docker stats
```

Ao terminar:

```bash
docker compose down
```

### Resultados medidos e comparação com o ranking oficial

Os números abaixo foram registrados em **23/09/2026**.

#### Resultado do nosso projeto

Este é o resultado que importa para acompanhar a evolução do repositório:

| Medição local | Resultado do nosso projeto | Como foi medido |
| --- | ---: | --- |
| Testes Go | 7 pacotes aprovados | `go test ./...` dentro de `golang:1.24` |
| Análise estática | aprovada | `go vet ./...` |
| Busca KNN | `18,306 ms/op` | força bruta sobre 1.000.000 referências |
| Alocações da busca | `720 B/op`, `11 allocs/op` | benchmark `BenchmarkBruteForceTopKOfficial` |
| Carga HTTP | `19,34 req/s` | 200 requisições concorrentes pelo Nginx |
| Latência HTTP | p50 `704,05 ms`; p95 `1.499,67 ms`; p99 `1.798,11 ms` | cliente Python local, concorrência 16 |
| Falhas HTTP | `0/200` | 200 respostas HTTP 200 |
| Distribuição | `100/100` por instância | 100 requisições em `api-1` e 100 em `api-2` |
| Memória observada | aproximadamente `58–63 MiB` por API | limite configurado de `120 MiB` por instância |

Com isso, o número de latência HTTP atual do nosso projeto é **p99 de
`1.798,11 ms` no teste local**. O tempo específico da busca KNN é
**`18,306 ms/op`**.

Como ainda não enviamos este projeto para a competição, ele **não possui p99,
score ou posição oficial no site**.

#### Referência do ranking oficial

| Submissão de referência | p99 | Falhas | Score |
| --- | ---: | ---: | ---: |
| 1º lugar: `rafaelcoelhox-detecta-fraude` | `0,4227 ms` | 0% | `6.000` |
| 9º lugar: `rinha-backend-2026-go` | `0,5068 ms` | 0% | `6.000` |

O benchmark local da busca mede somente o KNN por força bruta. Já o p99 oficial
é medido pelo ambiente da competição e inclui implementações muito otimizadas,
como índices vetoriais especializados, SIMD e servidores HTTP customizados.
Por isso, o nosso p99 local e o p99 oficial **não são uma comparação direta de
latência**; o valor oficial serve apenas como referência de objetivo.

Na prática, o resultado atual significa:

- funcionalmente, o projeto está passando nos testes unitários e na carga local;
- a memória observada ficou em aproximadamente `58–63 MiB` por API, abaixo do
  limite local de `120 MiB` por instância;
- em performance, ainda há espaço para substituir a busca por força bruta por
  um índice vetorial mais eficiente antes de comparar o projeto com o ranking.

Fonte dos resultados oficiais: [classificação final da Rinha de Backend 2026](https://rinhadebackend.com.br/).

### Edições anteriores

- [**2025** — Payment Processor](https://github.com/zanfranceschi/rinha-de-backend-2025)
- [**2024** — Crébitos (controle de concorrência)](https://github.com/zanfranceschi/rinha-de-backend-2024-q1)
- [**2023** — CRUD de Pessoas](https://github.com/zanfranceschi/rinha-de-backend-2023-q3)

### Redes sociais
- [Website Oficial](https://rinhadebackend.com.br/)
- [Discord](https://discord.gg/Eca6gJba8R)
- [X / Twitter](https://x.com/rinhadebackend)
- [LinkedIn](https://www.linkedin.com/company/108194083)
- [Bluesky](https://bsky.app/profile/rinhadebackend.bsky.social)

---

## English

**Rinha de Backend** is a friendly competition where you build a backend under CPU, memory, and architecture constraints. Each edition has a different theme – this one is **fraud detection using vector search**.

**Full challenge documentation:** [**docs/en/README.md**](./docs/en/README.md)

### Previous editions

- [**2025** — Payment Processor](https://github.com/zanfranceschi/rinha-de-backend-2025)
- [**2024** — Crébitos (concurrency control)](https://github.com/zanfranceschi/rinha-de-backend-2024-q1)
- [**2023** — People CRUD](https://github.com/zanfranceschi/rinha-de-backend-2023-q3)

### Social media

- [Official Website](https://rinhadebackend.com.br/)
- [Discord](https://discord.gg/Eca6gJba8R)
- [X / Twitter](https://x.com/rinhadebackend)
- [LinkedIn](https://www.linkedin.com/company/108194083)
- [Bluesky](https://bsky.app/profile/rinhadebackend.bsky.social)
