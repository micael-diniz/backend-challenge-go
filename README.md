# Serviço de carteira Jungle

Este serviço abre carteiras de jogadores e aplica `BET`, `WIN`, `LOSS`, `REFUND` e `ROLLBACK` por HTTP e por SQS. Cada comportamento saiu de um teste que falhava, escrito a partir dos requisitos em [CHALLENGE.md](CHALLENGE.md). O domínio guarda o dinheiro, o estado da carteira e as transições da aposta. O PostgreSQL implementa as portas de repositório. As decisões, inclusive locks, idempotência e o que ficou sem fazer, estão em [ARCHITECTURE.md](ARCHITECTURE.md).

## Pré-requisitos

- Go 1.27.0
- Docker com Compose

Copie o exemplo local. Se alterar o arquivo, deixe-o fora do Git:

```sh
cp .env.example .env
```

Os valores de `.env.example` são placeholders locais, da mesma classe que a senha do banco no Compose.

## Ambiente

| Variável | Função |
| --- | --- |
| `DATABASE_URL` | String de conexão do PostgreSQL |
| `HTTP_ADDR` | Endereço em que a API escuta |
| `MIGRATIONS_DIR` | Diretório dos arquivos `*.up.sql` |
| `OIDC_ISSUER` | Issuer do realm no Keycloak |
| `WALLET_CLIENT_ID` | Cliente autorizado nas rotas de carteira. Valor local: `wallet-service` |
| `SQS_ENDPOINT` | Endpoint compatível com SQS. Obrigatório. Valor local: `http://127.0.0.1:4566` |
| `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_PORT` | Banco do Compose. Porta no host, por padrão: `55432` |

`github.com/joho/godotenv` carrega `.env` e não substitui uma variável que já esteja definida. O Compose define `DATABASE_URL`, `HTTP_ADDR` e `MIGRATIONS_DIR` para o container da API. Esse container usa a rede do host e alcança o PostgreSQL pelo socket Unix compartilhado. Um processo no host pode usar `127.0.0.1:55432`, como em `.env.example`.

## Execução

```sh
docker compose up --build
```

Sobe:

- PostgreSQL `17.6-alpine`
- Keycloak `26.2.5`, que importa `deploy/keycloak/jungle-realm.json` na inicialização
- LocalStack `4.9.2` com SQS em `127.0.0.1:4566`
- a API em `127.0.0.1:8080`

Na subida, a API aplica as migrations `*.up.sql` pendentes e cria as filas `wager-transactions.fifo`, `wager-transactions-dlq.fifo` e `wallet-events.fifo`. A fila de apostas reenvia para a DLQ depois de 5 recebimentos. O visibility timeout é de 30 segundos. A espera do receive é de 1 segundo.

Pare com Ctrl+C. O processo cancela o consumidor, a varredura de referências pendentes e o publicador da outbox, e em seguida encerra o HTTP. Uma mensagem já recebida e ainda não apagada volta a ficar visível depois do visibility timeout.

Uma segunda instância usa o mesmo banco e as mesmas filas, com outro endereço:

```sh
HTTP_ADDR=127.0.0.1:8082 go run ./cmd/api
```

O publicador da outbox trava uma linha devida com `FOR UPDATE SKIP LOCKED`. O segundo processo publica outra linha, ou o mesmo `eventId`, se a reserva do primeiro sofrer rollback.

## Autenticação

O Keycloak provisiona dois clientes locais:

| Cliente | Segredo | Chamadas permitidas |
| --- | --- | --- |
| `wallet-service` | `wallet-service-secret` | Abrir carteira, ler qualquer carteira ou transação, reconciliar, ler métricas |
| `provider-a` | `provider-a-secret` | Enviar e ler somente as próprias transações de aposta |

Peça um token:

```sh
curl -s -X POST http://127.0.0.1:8081/realms/jungle/protocol/openid-connect/token \
  -d grant_type=client_credentials \
  -d client_id=wallet-service \
  -d client_secret=wallet-service-secret
```

Envie `Authorization: Bearer <access_token>` em todas as rotas, exceto `GET /health/live` e `GET /health/ready`. Token ausente ou inválido responde `401` `{"error":"unauthorized"}`. Um provedor chamando uma rota de carteira responde `403` `{"error":"forbidden"}`.

## Exemplos de chamada

Abra uma carteira com o token interno:

```sh
curl -s -X POST http://127.0.0.1:8080/wallets \
  -H "Authorization: Bearer $WALLET_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"playerId":"0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1","initialBalance":{"amount":"1000.00","currency":"BRL"}}'
```

Uma abertura positiva devolve a versão `1` e grava um `OPENING` processado, um crédito no ledger e dois eventos de outbox ainda não publicados. Uma abertura com saldo zero grava somente a carteira. O mesmo jogador e a mesma moeda de novo respondem `409` `wallet_already_exists`.

Faça uma aposta com o token do provedor. O header `Idempotency-Key` é obrigatório, e o servidor não o substitui:

```sh
curl -s -X POST http://127.0.0.1:8080/wagering/transactions \
  -H "Authorization: Bearer $PROVIDER_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: provider-a:transaction-123' \
  -d '{"providerId":"provider-a","externalTransactionId":"transaction-123","playerId":"0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1","walletId":"<wallet id>","roundId":"round-987","gameId":"fortune-chimp","kind":"BET","money":{"amount":"25.00","currency":"BRL"}}'
```

Uma aposta processada responde `200` com `transactionId`, `PROCESSED`, o saldo resultante e `idempotentReplay`. A mesma chave e os mesmos campos de negócio devolvem o saldo original, com `idempotentReplay: true`. A mesma chave com outro corpo responde `409` `idempotency_conflict`.

`WIN`, `REFUND` e `ROLLBACK` acrescentam `referenceExternalTransactionId`. Se essa referência ainda não foi processada, a resposta é `202` com `PENDING_REFERENCE` e sem saldo. Saldo insuficiente, referência incompatível e uma segunda reversão respondem `422` com um código estável em `error`. A rejeição gravada pode ser lida depois.

```sh
curl -s http://127.0.0.1:8080/wallets/<wallet id> \
  -H "Authorization: Bearer $WALLET_TOKEN"

curl -s 'http://127.0.0.1:8080/wallets/<wallet id>/ledger?limit=50' \
  -H "Authorization: Bearer $WALLET_TOKEN"

curl -s http://127.0.0.1:8080/wagering/transactions/<transaction id> \
  -H "Authorization: Bearer $PROVIDER_TOKEN"

curl -s -X POST http://127.0.0.1:8080/wallets/<wallet id>/reconciliation \
  -H "Authorization: Bearer $WALLET_TOKEN"
```

Depois da abertura de `1000.00` e da aposta de `25.00`, a reconciliação devolve saldo armazenado `975.00`, saldo calculado `975.00`, diferença `0.00`, `consistent: true` e `checkedEntries: 2`. Ela não altera o saldo armazenado.

```sh
curl -s http://127.0.0.1:8080/health/live
curl -s http://127.0.0.1:8080/health/ready
curl -s http://127.0.0.1:8080/metrics -H "Authorization: Bearer $WALLET_TOKEN"
```

`/health/live` devolve `{"status":"live"}`. `/health/ready` devolve `{"status":"ready"}` quando o PostgreSQL e a fila de apostas respondem, e `503` `{"status":"not_ready"}` caso contrário. `/metrics` é o JSON publicado pelo `expvar`.

Uma aposta por SQS usa os mesmos campos de negócio. `MessageGroupId` é o id da carteira e `MessageDeduplicationId` é o `messageId` do envelope. O corpo segue o exemplo `WagerTransactionRequested` em [CHALLENGE.md](CHALLENGE.md). Os eventos da outbox vão para `wallet-events.fifo`, com `MessageGroupId` no id do agregado e `MessageDeduplicationId` no id do evento.

## Migrations

A inicialização aplica cada `migrations/*.up.sql` que ainda não está em `schema_migrations`, em ordem de nome:

| Versão | Mudança |
| --- | --- |
| `000001_wallets` | Carteiras |
| `000002_wager_transactions` | Transações de aposta, formato de abertura |
| `000003_pending_bet` | Aposta pendente |
| `000004_external_identity` | Provedor e id externo |
| `000005_processed_bet` | Aposta processada |
| `000006_wallet_ledger` | Ledger append-only |
| `000007_outbox` | Outbox |
| `000008_rejected_bet` | Aposta rejeitada |
| `000009_processed_loss` | Loss processado |
| `000010_processed_win` | Win processado |
| `000011_processed_reversal` | Reversão processada |
| `000012_pending_reference` | Referência pendente |
| `000013_inbox` | Inbox |

O processo não tem comando de down. Para reverter, pare a API e aplique os `*.down.sql` de `000013_inbox` até `000001_wallets`. Depois de cada arquivo, apague essa versão em `schema_migrations`. O `*.down.sql` correspondente é o rollback exercitado pelos testes de integração.

## Testes

Testes unitários, o detector de corrida e o `go vet` não precisam de Docker. Os testes de integração usam a build tag `integration` e não entram em `go test ./...`.

```sh
go test ./...
go test -race ./...
go vet ./...
gofmt -l .
```

Suba as dependências e rode a integração contra os containers reais de PostgreSQL, Keycloak e LocalStack:

```sh
docker compose up -d postgres localstack keycloak
export TEST_DATABASE_URL='postgres://jungle:jungle@127.0.0.1:55432/jungle?sslmode=disable'
export SQS_ENDPOINT='http://127.0.0.1:4566'
go test -tags=integration -count=1 ./internal/infrastructure/postgres/ ./internal/infrastructure/sqs/ ./internal/interfaces/httpapi/
```

A suíte de PostgreSQL derruba e recria o schema `public`. Aponte `TEST_DATABASE_URL` para um banco que possa ser resetado. O container da API deve estar parado durante essa suíte, porque ele consome `wager-transactions.fifo` e leva a mensagem que o teste de SQS está esperando.

Testes dessa suíte que cobrem concorrência e recuperação:

- `TestFiftyParallelCopiesOfOneBetDebitOnce` envia a mesma aposta de `25.00` em 50 processos numa carteira de `100.00` e espera um débito e saldo `75.00`.
- `TestThreeProcessesKeepOneDebitWhenTwoBetsRace` disputa dois processos filhos, cada um apostando `80.00` numa carteira de `100.00`, e espera um único débito.
- `TestSeparateWalletsAcceptConcurrentBets` aposta em carteiras diferentes ao mesmo tempo.
- `TestThreeInstancesBetSeparateWallets` aposta `80.00` em três carteiras de `100.00`, cada uma num processo, e espera as três em `20.00`.
- `TestPostgresInboxDebitsABetOnceWhenTheMessageIsRedelivered` confirma uma aposta e verifica a segunda entrega.
- `TestPostgresRedeliversACommittedBetWhenDeleteFails` confirma a aposta, falha o delete da fila e confere que a reentrega não debita de novo.
- `TestPostgresHTTPAndSQSShareOneBet` envia a mesma aposta pelas duas entradas.
- `TestPostgresOutboxPublishesTheSameEventAfterALostConfirmation` e `TestPostgresOutboxLockLetsAnotherPublisherResumeAfterRollback` cobrem a outbox.
- `TestPostgresPendingReferenceResolvesAfterTheBetArrives` grava uma espera antes da aposta e continua o trabalho a partir de um novo valor de serviço.
- `TestANewProcessResolvesAPendingReference` grava a espera num processo e o processo seguinte coloca a aposta e resolve o crédito.

`go.mod` e `go.sum` fixam as dependências do módulo. Formate os arquivos Go com `gofmt` antes de enviar uma mudança.
