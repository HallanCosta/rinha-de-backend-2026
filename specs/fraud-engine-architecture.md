# Arquitetura do motor de detecção de fraude

Status: especificação de implementação. Este documento descreve a próxima etapa
do projeto; ele não implementa a lógica de fraude e não altera os arquivos de
runtime existentes.

## 1. Objetivo

Substituir o stub atual de `POST /fraud-score` por um motor organizado em
camadas que:

1. decodifica o payload definido em [`docs/br/API.md`](../docs/br/API.md);
2. transforma cada transação em um vetor de 14 dimensões;
3. busca os 5 vetores de referência mais próximos;
4. calcula `fraud_score` como a fração de vizinhos rotulados como fraude;
5. responde `approved = fraud_score < 0.6`.

A aplicação deve continuar expondo exatamente `GET /ready` e
`POST /fraud-score` na porta `9999`. O Nginx continua sendo apenas o load
balancer entre pelo menos duas instâncias da API.

As regras de negócio vêm exclusivamente de:

- [`API.md`](../docs/br/API.md), para o contrato HTTP e o payload;
- [`REGRAS_DE_DETECCAO.md`](../docs/br/REGRAS_DE_DETECCAO.md), para as 14
  dimensões, normalização, sentinela `-1` e decisão;
- [`DATASET.md`](../docs/br/DATASET.md), para os arquivos de referência;
- [`BUSCA_VETORIAL.md`](../docs/br/BUSCA_VETORIAL.md), para o conceito e as
  alternativas de busca.

## 2. Fora de escopo

Esta especificação não inclui:

- alteração imediata de `cmd/api/main.go`, `Dockerfile`, `docker-compose.yml` ou
  `nginx.conf`;
- criação de um modelo de machine learning treinado;
- uso dos payloads de teste como lookup ou referência;
- mudança do threshold fixo de `0.6`;
- criação de endpoints adicionais;
- decisão definitiva entre força bruta, ANN, banco vetorial ou outra técnica
  antes de medir correção, memória e latência;
- autenticação, persistência de transações recebidas ou atualização do dataset
  em tempo de execução.

O dataset de referência é estático durante o teste. Ele pode ser
pré-processado no build ou no startup, conforme permitido por
[`DATASET.md`](../docs/br/DATASET.md).

## 3. Estado atual do repositório

O repositório já possui:

- `cmd/api/main.go` com um servidor HTTP mínimo;
- `GET /ready` respondendo prontidão;
- `POST /fraud-score` retornando uma decisão fixa, apenas para testar a
  topologia;
- `Dockerfile` multi-stage para gerar um binário Go `linux/amd64`;
- `docker-compose.yml` com `api-1`, `api-2` e `load-balancer`;
- `nginx.conf` com round-robin entre as duas APIs;
- limites atuais de `0.60` CPU e `290 MB` somados.

O motor real deverá ser adicionado atrás do handler, mantendo `main.go` como
composition root: carregar objetos, conectar dependências e iniciar o servidor.

## 4. Estrutura de pastas proposta

```text
cmd/
└── api/
    └── main.go

internal/
├── http/
│   ├── handler.go
│   └── routes.go
├── model/
│   ├── transaction.go
│   ├── vector.go
│   └── decision.go
├── dataset/
│   ├── loader.go
│   ├── reference_store.go
│   └── metadata.go
├── vectorizer/
│   ├── vectorizer.go
│   └── dimensions.go
├── search/
│   ├── searcher.go
│   └── brute_force.go
└── rules/
    ├── engine.go
    └── decision.go

resources/
├── references.json.gz
├── mcc_risk.json
└── normalization.json
```

Os nomes são uma proposta de organização. A implementação pode dividir ou
combinar arquivos dentro de um pacote, mas deve preservar as responsabilidades
e os contratos descritos abaixo.

## 5. Responsabilidades por pacote e arquivo

### `cmd/api/main.go`

É o ponto de entrada e deve conter somente bootstrap da aplicação:

1. ler configuração de ambiente/flags e caminhos dos recursos;
2. carregar o dataset e os metadados;
3. construir o vectorizer, o searcher e o engine;
4. construir as rotas HTTP;
5. iniciar o servidor na porta `9999`.

`main.go` não deve conhecer fórmulas das 14 dimensões, distância entre vetores
ou regra de aprovação. Ele apenas conecta implementações concretas às
interfaces das camadas.

### `internal/http`

O nome do diretório segue a arquitetura proposta. Para evitar confusão com a
biblioteca padrão `net/http`, o pacote Go pode se chamar `httpapi`.

#### `handler.go`

Responsável por:

- aceitar `POST /fraud-score`;
- decodificar o JSON em `model.FraudRequest`;
- chamar uma interface de aplicação, sem calcular score diretamente;
- serializar somente `approved` e `fraud_score` na resposta de sucesso;
- traduzir erros de entrada e de infraestrutura para respostas HTTP
  convencionais, sem alterar o contrato de sucesso.

O handler não deve acessar arquivos, conhecer gzip ou iterar pelos 3 milhões de
referências.

#### `routes.go`

Responsável por montar o `http.Handler`:

- registrar exatamente `GET /ready`;
- registrar exatamente `POST /fraud-score`;
- conectar o estado de prontidão ao carregamento concluído do dataset.

O endpoint `/ready` só deve responder 2xx quando as dependências necessárias
para processar fraude estiverem disponíveis. O stub atual responde antes dessa
condição porque ainda não existe carregamento do motor.

### `internal/model`

Contém tipos de domínio sem dependência de HTTP, Docker ou algoritmo de busca.

#### `transaction.go`

Deve representar o payload da documentação: identificador, `transaction`,
`customer`, `merchant`, `terminal` e `last_transaction` como ponteiro/opção,
pois este último pode ser `null`.

Os timestamps representam strings ISO/UTC do contrato. Usar `time.Time` com o
decoder padrão é uma opção adequada, desde que o parsing continue aceitando o
formato documentado.

#### `vector.go`

Define o vetor lógico de 14 posições, mantendo a ordem da especificação. Uma
representação possível é um array fixo, e não um slice de tamanho variável:

```go
type Vector [14]float64
```

As posições 5 e 6 podem conter `-1` quando não existe `last_transaction`. Esse
valor não deve ser substituído por zero.

#### `decision.go`

Define os valores trocados entre as camadas, por exemplo:

```go
type Neighbor struct {
    Distance float64
    IsFraud  bool
}

type Decision struct {
    Approved   bool    `json:"approved"`
    FraudScore float64 `json:"fraud_score"`
}
```

O campo JSON `label` do dataset (`fraud` ou `legit`) pode ser convertido para
`IsFraud` durante o carregamento.

### `internal/dataset`

Responsável por abrir, validar e disponibilizar os dados estáticos:

- `references.json.gz`;
- `mcc_risk.json`;
- `normalization.json`.

#### `loader.go`

Deve fazer leitura incremental de `references.json.gz`. Não deve usar um
`json.Unmarshal` do arquivo inteiro para uma estrutura genérica, porque o
dataset descompactado tem aproximadamente 284 MB e o Compose atual limita cada
API a 120 MB.

O loader deve retornar erro explícito para arquivo ausente, gzip inválido,
JSON inválido, vetor com dimensão diferente de 14 ou label desconhecido.

#### `reference_store.go`

Expõe acesso somente leitura ao conjunto carregado. O formato interno precisa
ser compacto e evitar uma alocação Go por registro, como `[][]float64` ou um
mapa por transação.

O contrato lógico pode ser:

```go
type ReferenceStore interface {
    Len() int
    At(index int, dst *model.Vector) (isFraud bool, ok bool)
}
```

Esse contrato permite trocar a representação depois de medir memória e
latência. Possibilidades a serem comparadas incluem armazenamento plano,
quantização, arquivo pré-processado ou índice especializado. A representação
escolhida não pode mudar a ordem das 14 dimensões nem o significado do
sentinela `-1`.

#### `metadata.go`

Carrega e valida as constantes de `normalization.json` e o mapa MCC → risco de
`mcc_risk.json`. Quando o MCC não estiver no mapa, usa o fallback `0.5`, conforme
[`DATASET.md`](../docs/br/DATASET.md).

Esses dados são imutáveis depois do startup e podem ser compartilhados pelas
operações de vetorização da instância.

### `internal/vectorizer`

Transforma `model.FraudRequest` em `model.Vector`. É uma camada pura: não deve
ler HTTP nem acessar o dataset de referências.

#### `vectorizer.go`

Recebe a requisição e os metadados de normalização/MCC. Uma interface possível:

```go
type Vectorizer interface {
    Vectorize(request model.FraudRequest) (model.Vector, error)
}
```

#### `dimensions.go`

Pode concentrar funções pequenas como `clamp`, conversão de horário e
conversão de dia da semana. A ordem obrigatória é:

| Índice | Dimensão | Regra |
|---:|---|---|
| 0 | `amount` | `clamp(amount / max_amount)` |
| 1 | `installments` | `clamp(installments / max_installments)` |
| 2 | `amount_vs_avg` | `clamp((amount / customer.avg_amount) / amount_vs_avg_ratio)` |
| 3 | `hour_of_day` | hora UTC dividida por 23 |
| 4 | `day_of_week` | segunda-feira 0 até domingo 6, dividido por 6 |
| 5 | `minutes_since_last_tx` | minutos desde a transação anterior, normalizados; `-1` se ausente |
| 6 | `km_from_last_tx` | distância normalizada; `-1` se ausente |
| 7 | `km_from_home` | `clamp(km_from_home / max_km)` |
| 8 | `tx_count_24h` | `clamp(tx_count_24h / max_tx_count_24h)` |
| 9 | `is_online` | `1` ou `0` |
| 10 | `card_present` | `1` ou `0` |
| 11 | `unknown_merchant` | `1` se o comerciante não for conhecido, senão `0` |
| 12 | `mcc_risk` | risco do MCC, com fallback `0.5` |
| 13 | `merchant_avg_amount` | `clamp(avg_amount / max_merchant_avg_amount)` |

As fórmulas e constantes completas estão em
[`REGRAS_DE_DETECCAO.md`](../docs/br/REGRAS_DE_DETECCAO.md). A tabela acima é
um mapa de responsabilidades, não uma nova regra.

### `internal/search`

Responsável apenas por encontrar os vizinhos mais próximos. Não decide se a
transação será aprovada.

#### `searcher.go`

Define o contrato que o engine usará:

```go
type Searcher interface {
    TopK(ctx context.Context, query model.Vector, k int) ([]model.Neighbor, error)
}
```

O retorno precisa estar ordenado por distância crescente e conter `k` vizinhos
quando o dataset tiver referências suficientes.

#### `brute_force.go`

Implementa o baseline correto para uma fixture pequena e para benchmarks:

1. percorre as referências;
2. calcula a distância euclidiana nas 14 dimensões;
3. mantém somente os melhores `k`, sem ordenar todos os 3 milhões de itens;
4. retorna distância e label de cada vizinho.

Para o dataset completo, a escolha final pode ser força bruta otimizada, ANN,
índice pré-processado ou outra técnica permitida. A interface deve permanecer
estável para que a troca não altere o handler nem as regras.

### `internal/rules`

Orquestra a decisão de negócio usando um vectorizer e um searcher.

#### `engine.go`

Coordena uma requisição:

1. chama o vectorizer;
2. chama `TopK` com `k = 5`;
3. passa os vizinhos para a decisão;
4. devolve `model.Decision`.

Uma interface de entrada possível para o handler é:

```go
type Scorer interface {
    Score(ctx context.Context, request model.FraudRequest) (model.Decision, error)
}
```

#### `decision.go`

Aplica somente as regras documentadas:

```text
fraud_score = quantidade de fraudes entre os 5 vizinhos / 5
approved = fraud_score < 0.6
```

Não deve aplicar heurísticas adicionais, consultar o ID da transação, fazer
lookup dos payloads de teste ou alterar o threshold.

## 6. Contratos entre as camadas

O sentido das dependências deve ser:

```text
cmd/api
  └── monta ──> httpapi ──> rules ──> vectorizer
                                  └──> search ──> dataset

model fica na base e não depende das camadas externas.
```

Regras práticas:

- `model` não importa `net/http`;
- `vectorizer` não conhece JSON bruto nem headers HTTP;
- `search` não conhece `approved`;
- `rules` não abre arquivos;
- `httpapi` não conhece o formato do dataset;
- `main.go` pode importar todos os pacotes concretos para fazer a composição;
- dependências de infraestrutura devem ser injetadas por interfaces pequenas.

## 7. Fluxo de startup

O startup futuro deve seguir esta ordem:

1. ler caminhos/configuração dos recursos;
2. carregar `normalization.json`;
3. carregar `mcc_risk.json`;
4. abrir e pré-processar `references.json.gz` para uma estrutura somente
   leitura;
5. construir `vectorizer`, `searcher` e `rules.Engine`;
6. montar o router HTTP;
7. marcar a aplicação como pronta;
8. escutar em `:9999`.

Se o dataset não carregar, a instância não deve anunciar prontidão. O erro deve
aparecer no log e impedir um processo aparentemente saudável com respostas
incorretas.

## 8. Fluxo por requisição

```text
POST /fraud-score
        │
        ▼
httpapi decodifica FraudRequest
        │
        ▼
rules.Engine.Score
        │
        ├── vectorizer.Vectorize
        │       └── Vector[14]
        │
        ├── searcher.TopK(query, 5)
        │       └── 5 Neighbor
        │
        └── rules.Decide
                └── Decision
        │
        ▼
JSON {"approved": ..., "fraud_score": ...}
```

O Nginx fica fora desse fluxo de negócio: ele apenas escolhe `api-1` ou
`api-2` e encaminha a requisição.

## 9. Estratégia de testes

### `internal/model`

- decodificar o payload completo de `API.md`;
- decodificar `last_transaction: null`;
- verificar que os campos JSON têm os nomes do contrato;
- verificar que `Decision` serializa exatamente os dois campos esperados.

### `internal/dataset`

- carregar fixtures pequenas em JSON e JSON gzipado;
- rejeitar dimensão diferente de 14;
- rejeitar label diferente de `fraud` ou `legit`;
- validar as constantes de `normalization.json`;
- validar MCC conhecido e fallback `0.5`;
- verificar que o loader não mantém uma árvore JSON genérica inteira.

### `internal/vectorizer`

- reproduzir os vetores dos exemplos legítimo e fraudulento da documentação;
- comparar floats com tolerância explícita, sem igualdade frágil;
- testar `last_transaction: null` e os dois sentinelas `-1`;
- testar clamp abaixo de zero e acima de um;
- testar hora e dia da semana em UTC;
- testar comerciante conhecido e desconhecido;
- testar MCC ausente no mapa.

### `internal/search`

- usar seis ou mais referências artificiais com distâncias conhecidas;
- verificar ordenação crescente;
- verificar que somente os melhores cinco são retornados;
- verificar label e distância de cada vizinho;
- testar cancelamento de `context.Context`;
- comparar a implementação otimizada com um baseline simples em fixture pequena.

### `internal/rules`

- 0 de 5 fraudes → score `0.0`, aprovado;
- 2 de 5 fraudes → score `0.4`, aprovado;
- 3 de 5 fraudes → score `0.6`, negado;
- 5 de 5 fraudes → score `1.0`, negado;
- erro do vectorizer e erro do searcher propagados ao chamador.

### `internal/http`

- `/ready` antes e depois do estado de prontidão;
- payload válido encaminhado uma vez ao scorer;
- resposta de sucesso com os campos corretos;
- JSON inválido sem panic;
- método/path não previstos não registrados como endpoints da aplicação.

### Integração

Depois dos testes unitários, executar:

```bash
go test ./...
go vet ./...
docker compose config -q
docker compose up --build -d
curl -i http://localhost:9999/ready
docker compose down
```

O teste de integração deve confirmar que o Nginx continua alternando entre as
duas instâncias, mas não deve depender do header temporário
`X-API-Instance` como parte do contrato da Rinha.

## 10. Sequência incremental de implementação

1. **Modelos e contratos**: adicionar `internal/model` e testes de JSON.
2. **Vectorizer**: implementar as 14 dimensões usando fixtures dos docs; ainda
   sem dataset completo.
3. **Loader pequeno**: carregar metadados e uma fixture de referências.
4. **Busca baseline**: implementar brute force e validar top-5 conhecido.
5. **Rules engine**: conectar vectorizer, searcher e decisão `0.6`.
6. **Handler**: substituir o stub por uma dependência `Scorer`; manter o
   handler sem fórmulas.
7. **Bootstrap**: mover a composição para `cmd/api/main.go` e só então alterar
   o comportamento de prontidão.
8. **Dataset real**: adicionar os arquivos de referência e medir startup,
   memória e latência.
9. **Otimização**: trocar a representação/algoritmo de busca somente se os
   benchmarks mostrarem necessidade; preservar testes de correção.
10. **Containerização final**: incluir o índice/recursos no processo de build ou
    startup, revisar os limites do Compose e testar as duas instâncias.

Cada etapa deve deixar `go test ./...` verde. Não vale otimizar a busca antes de
ter um baseline correto que possa ser usado como comparação.

## 11. Memória e limites da Rinha

O `references.json.gz` tem aproximadamente 16 MB comprimido e 284 MB
descompactado. Portanto, não é compatível com simplesmente desserializar o
arquivo inteiro em `[]Reference` contendo slices e strings dentro de cada API.

O desenho precisa considerar que o Compose atual reserva:

```text
api-1:          120 MB
api-2:          120 MB
load-balancer:   50 MB
total:          290 MB
```

Antes da implementação final, medir pelo menos:

- memória de startup e memória após o primeiro request;
- tempo para carregar/preprocessar as referências;
- latência p50/p95/p99;
- resultado do top-5 contra o baseline;
- comportamento com as duas APIs simultâneas.

O load balancer não pode receber a responsabilidade de reduzir memória,
vetorizar ou consultar o dataset. Se for necessário compartilhar dados ou
introduzir outro serviço, a decisão deve ser validada contra
[`ARQUITETURA.md`](../docs/br/ARQUITETURA.md) e os limites totais do Compose.

## 12. Paralelo com Node.js/TypeScript

| Go nesta arquitetura | Node.js/TypeScript típico | Ideia principal |
|---|---|---|
| `cmd/api/main.go` | `src/server.ts` ou `src/index.ts` | bootstrap e composição das dependências |
| `internal/http` | routes/controllers | protocolo HTTP e tradução de entrada/saída |
| `internal/model` | `types/`, `interfaces/`, domain types | tipos do domínio |
| `internal/dataset` | repository/loader/adapter | leitura e preparação dos dados |
| `internal/vectorizer` | service/pure function | transformação determinística do payload |
| `internal/search` | adapter de banco/serviço de busca | consulta dos vizinhos mais próximos |
| `internal/rules` | use case/application service | orquestração e decisão de negócio |
| `go.mod` | `package.json` | módulo e dependências |
| `go test ./...` | `npm test`/`pnpm test` | execução dos testes |
| `error` retornado explicitamente | `throw`/`try...catch` | tratamento explícito de falhas |
| goroutine e `context.Context` | async/Promise e AbortSignal | concorrência e cancelamento |
| slice `[]T` | array `T[]` | coleção ordenada |
| map `map[K]V` | `Map<K, V>` ou objeto | associação chave-valor |

Algumas diferenças importantes para quem vem de TypeScript:

- uma `struct` Go descreve dados concretos; uma `interface` Go descreve
  comportamento, não apenas o formato de um objeto;
- o compilador verifica tipos, imports e muitos erros antes de executar;
- não existe `node_modules` em runtime: o Docker compila um binário e a imagem
  final contém somente o executável;
- funções retornam valores e erros explicitamente, em vez de depender sempre de
  exceções;
- `go test` faz parte da ferramenta padrão da linguagem;
- `main.go` não precisa conter toda a aplicação: os pacotes `internal` são a
  forma de separar o domínio e impedir que detalhes HTTP vazem para a busca.

## 13. Critério de conclusão desta arquitetura

Consideramos a organização pronta quando:

- `cmd/api/main.go` apenas monta dependências e inicia o servidor;
- o handler não contém fórmulas ou loop sobre referências;
- o vectorizer passa pelos testes dos exemplos da documentação;
- o searcher retorna os 5 vizinhos corretos em fixtures;
- o engine aplica somente `fraud_score = fraudes / 5` e `score < 0.6`;
- o loader funciona com os arquivos reais sem carregar JSON genérico inteiro;
- os testes unitários e de integração passam;
- o Compose continua com load balancer, duas APIs, `bridge`, `linux/amd64`,
  porta `9999` e limites dentro da Rinha.
