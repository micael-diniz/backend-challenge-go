# Arquitetura

Os requisitos em [CHALLENGE.md](CHALLENGE.md) foram a origem de cada comportamento. Um teste descrevia o resultado exigido antes do código de produção. Esse código entrava só até o teste passar. As regras de domínio foram provadas antes de existir um repositório. O PostgreSQL então provou as mesmas regras numa transação real, com constraint e com reinício. O adaptador em memória permanece para os testes unitários de HTTP. Ele não é a evidência de durabilidade, de idempotência entre processos nem de recuperação.

## Domínio e repositórios

`internal/domain` guarda `Money`, `Wallet`, `WagerTransaction`, `WalletLedgerEntry` e os quatro tipos de evento. Não importa repositório, HTTP nem driver de banco. Os métodos devolvem erros sentinela. Não escrevem log.

`internal/application` guarda os casos de uso e as portas de repositório: abertura de carteira, aposta, leituras, inbox e outbox. O domínio não depende dessas interfaces. `internal/infrastructure/postgres` implementa as portas com `pgx` e com o SQL escrito na store. Não há ORM nem framework de migration. Um ORM esconderia o predicado do `UPDATE` da carteira e a transação única que grava a carteira, a aposta, o ledger e a outbox.

O Fx liga uma store PostgreSQL a cada porta de que o processo precisa. O HTTP fica em `internal/interfaces/httpapi`. O cliente SQS fica em `internal/infrastructure/sqs`. O loop do consumidor, a varredura de referências pendentes e a varredura da outbox ficam em `cmd/api`.

## Dinheiro

Os valores são `int64` com sinal, em unidades menores, gravados em `BIGINT`. `100.00 BRL` é `10000`. O maior valor que o parse aceita é `92233720368547758.07`, o último centavo que cabe em `int64`. O centavo seguinte é overflow, não um saldo truncado. Parse, conta, JSON e SQL não usam `float32` nem `float64`.

A escala externa é exatamente duas casas. `25`, `25.0`, `25.000` e notação científica são recusados, sem arredondar. A única forma equivalente aceita é `-0.00`, que vira `0.00`, e o código da moeda, que vira maiúsculas. `brl` fica `BRL`. `BRL` e `USD` são as moedas do schema. Essa forma canônica é a que entra no hash de idempotência: o hash vê `0.00` e `BRL`, não o texto cru. A representação com sinal existe para a diferença da reconciliação poder ser negativa. Quem recusa saldo negativo é a regra da carteira, não o próprio `Money`.

## Transações

Uma abertura positiva confirma, na mesma transação, a carteira na versão `1`, um `OPENING` processado, um crédito no ledger e as linhas de outbox `WagerTransactionProcessed` e `WalletBalanceChanged`. Uma abertura com saldo zero confirma só a carteira.

A aplicação não abre uma transação para repartir entre repositórios. Cada método da store PostgreSQL é a fronteira: um `BEGIN` e um `COMMIT`. `SaveProcessedBet` grava, nessa ordem, a carteira, a aposta, o lançamento e as duas linhas de outbox. `SaveProcessedLoss` grava a perda e `WagerTransactionProcessed`, sem lançamento e sem `UPDATE` da carteira.

Uma aposta síncrona carrega a carteira, monta a aposta `PENDING` só na memória, aplica a transição de domínio e confirma o estado terminal. Não há commit intermediário de `PENDING`. O estado durável de espera é `PENDING_REFERENCE`. Uma falha faz rollback dessa transação. `LOSS` grava a aposta processada e não muda o saldo. `WIN` credita o próprio valor; sem `referenceExternalTransactionId`, o crédito acontece na hora, e o valor não precisa ser o da aposta citada. O `POST` de uma `BET` pode trazer `referenceExternalTransactionId`; o construtor da aposta não copia esse campo, a linha gravada fica sem referência e o saldo é debitado. `REFUND` credita o valor exato de uma `BET` processada. `ROLLBACK` de `BET` credita; `ROLLBACK` de `WIN` ou de `REFUND` debita.

O índice único `(provider_id, reference_external_transaction_id)` cobre `REFUND` e `ROLLBACK` processados juntos. Uma referência recebe uma reversão processada, não uma de cada tipo. `REFUND` da aposta e `ROLLBACK` da mesma aposta não devolvem o débito duas vezes: o segundo é `duplicate_reversal`. Desfazer o reembolso é um `ROLLBACK` cujo `referenceExternalTransactionId` é o id externo do `REFUND`, não o da aposta. Esse movimento debita o mesmo valor. Se o saldo não cobre o débito, a linha fica `REJECTED` com `REVERSAL_INSUFFICIENT_FUNDS`, distinto de `INSUFFICIENT_FUNDS` de uma aposta.

O ledger confere a equação de crédito e débito no banco e recusa `UPDATE` e `DELETE` com SQLSTATE `55000`.

## Idempotência

O header HTTP `Idempotency-Key` é gravado como veio. O hash do payload é SHA-256 do JSON canônico de `encoding/json`, com as chaves do objeto ordenadas. Os campos no hash são `providerId`, `externalTransactionId`, `playerId`, `walletId`, `roundId`, `gameId`, `kind` e `money`. `WIN`, `REFUND` e `ROLLBACK` também incluem `referenceExternalTransactionId`. A chave de idempotência e os metadados de transporte ficam de fora. HTTP e SQS usam esse mesmo hash.

A mesma chave e o mesmo hash devolvem o resultado gravado, inclusive o saldo original. A mesma chave com outro hash é conflito. O mesmo `(providerId, externalTransactionId)` com outra chave também é conflito. Se a linha é confirmada entre a consulta pela chave e a consulta pelo id externo, a mesma chave e o mesmo hash ainda devolvem o resultado gravado.

O SQS acrescenta o `messageId` do envelope como identidade da inbox. O mesmo id de mensagem e o mesmo hash não chamam de novo o caminho do dinheiro. O mesmo id com outro hash permanece na fila e não é apagado. A linha da inbox é escrita depois do commit da aposta. Se o processo parar entre essas duas escritas, a chave de idempotência já impede um segundo movimento, e a entrega seguinte grava a inbox.

## Máquina de estados

`PENDING` só existe na memória de uma operação síncrona. O commit grava `PROCESSED` ou `REJECTED`. `PENDING_REFERENCE` é o único estado não terminal confirmado no banco, e outro processo o retoma. `PROCESSED`, `REJECTED` e `FAILED` não transitam de novo. O replay lê a linha e não reaplica o débito.

`FAILED`, com `PERMANENT_INFRASTRUCTURE_FAILURE`, existe no domínio e não é gravado pelo HTTP nem pelo consumidor. Tentativas de versão esgotadas respondem `500` sem linha nova. Mensagem inválida vai para a DLQ, também sem linha `FAILED`.

Falha transitória volta a ser tentada: conflito de versão, até 3 vezes no pedido; publicação da outbox, com espera de 1s, 2s, 4s, no máximo um minuto; recebimento SQS, com visibility de 30 segundos. Falha permanente de negócio é a linha `REJECTED`. Falha permanente da mensagem é o redrive depois de 5 recebimentos.

## Locks e concorrência

A coordenação é por carteira, não por um lock global. A linha é atualizada com `WHERE id = $1 AND version = $2`. É controle otimista com retry limitado, mais o predicado atômico no banco. Um `SELECT FOR UPDATE` não foi usado: o predicado continua valendo se o processo morre no meio, e duas carteiras não disputam a mesma linha. Zero linhas atualizadas são conflito de versão. A aplicação tenta de novo esse conflito até 3 vezes no mesmo processo. Depois disso o pedido falha e a linha confirmada por outro escritor permanece. Uma violação única em `wager_transactions_provider_external_id_unique` ou `wager_transactions_provider_idempotency_key_unique` desfaz a tentativa, e a tentativa seguinte lê a aposta gravada. Dois processos apostando `80.00` sobre `100.00` produzem um débito e uma rejeição por saldo insuficiente. A rejeição fica gravada.

O worker da outbox trava uma linha devida com `FOR UPDATE SKIP LOCKED`. Uma reserva que sofre rollback deixa a linha para outro publicador. Uma confirmação de publicação perdida reenvia o mesmo id de evento. Uma linha com `published_at` preenchido não é enviada de novo.

A reconciliação lê a carteira e o ledger numa transação `REPEATABLE READ, READ ONLY`. O saldo calculado começa em zero, soma créditos e subtrai débitos. `difference` é o saldo armazenado menos o calculado. O saldo armazenado não é alterado.

## Referências pendentes

Um `WIN`, `REFUND` ou `ROLLBACK` que cita um `externalTransactionId` ainda inexistente, ou cuja referência ainda está `PENDING_REFERENCE`, fica `PENDING_REFERENCE`. A carteira não se move. A primeira nova tentativa é um segundo depois. Cada espera seguinte dobra, e a espera expira um minuto depois do pedido. No prazo, a linha vira `REJECTED` com `REFERENCE_NOT_FOUND` e o evento `WagerTransactionRejected`. Uma referência já `REJECTED`, ou incompatível em jogador, carteira, moeda ou rodada, responde `422` na hora e não grava linha nova. `REFUND` e `ROLLBACK` sem referência, ou com valor diferente do citado, também respondem `422` `invalid_reference` sem linha nova. Reversão duplicada e reversão sem saldo gravam a linha `REJECTED`.

O agendamento está no PostgreSQL. A API consulta as linhas devidas a cada segundo, então um processo novo continua uma espera que um processo anterior gravou.

## Inbox, outbox e encerramento

Rejeições de negócio já confirmadas são terminais, e o consumidor apaga essas mensagens. Um erro de banco deixa a mensagem para outra tentativa. Um envelope inválido não é apagado. Depois de 5 recebimentos, a fila o reenvia para `wager-transactions-dlq.fifo`.

A publicação da outbox é um worker separado. Ela corre depois do commit da transação de origem. Um erro de publicação aumenta as tentativas e espera 1s, 2s, 4s, no máximo um minuto. O destino provisionado é `wallet-events.fifo`. `MessageGroupId` é o id do agregado e `MessageDeduplicationId` é o `eventId`. Este processo publica nessa fila e não a consome. O contrato da mensagem é o envelope do evento: `eventId`, `eventType`, `aggregateId`, `correlationId`, `causationId` quando existe, `occurredAt`, `version` e `data`.

Na entrada, `wager-transactions.fifo` usa `MessageGroupId` no id da carteira e `MessageDeduplicationId` no `messageId`. A DLQ é `wager-transactions-dlq.fifo`. O visibility timeout é 30 segundos, o receive espera 1 segundo, e o redrive ocorre no quinto recebimento. Envelope inválido não é apagado até esse redrive. HTTP e SQS chamam o mesmo `Apply`. A linha da inbox não entra no `COMMIT` da aposta: ela é gravada depois. Se o processo para no meio, a chave de idempotência já impede o segundo movimento, e a entrega seguinte grava a inbox.

`SIGTERM` cancela o contexto dos workers e depois encerra o servidor HTTP. O consumidor para de pedir mensagens. Uma mensagem já recebida e não apagada volta a ficar visível depois de 30 segundos. Um commit concluído antes do delete fica protegido pela chave de idempotência e, na entrega seguinte, pela inbox.

Os eventos exigidos são tipos concretos: `WagerTransactionProcessed`, `WagerTransactionRejected`, `WalletBalanceChanged` e `WagerTransactionPendingReference`. Os construtores definem o tipo e a versão do evento. Os payloads usam timestamp UTC em RFC 3339 e dinheiro em string decimal.

## Autenticação e autorização

O IdP é o Keycloak do Compose, realm `jungle`. A seção 2 recomenda esse IdP e proíbe cadastro de senha e token assinado pela própria API. Os chamadores são serviços, então o grant é `client_credentials`: `wallet-service` para a casa e `provider-a` para o provedor. Não há grant de senha de jogador.

A validação usa `github.com/coreos/go-oidc/v3`. Na primeira requisição a API descobre o issuer, confere assinatura pela JWKS, issuer e expiração. O cliente autenticado é o claim `azp`, não um campo do corpo. Os tokens deste realm trazem `aud=account`, que não é a audiência da API, então `SkipClientIDCheck` fica ligado. `azp` vazio é `401`, como token ausente ou assinatura inválida.

O modelo de permissão é o próprio `azp`. Carteira, reconciliação e `/metrics` exigem `WALLET_CLIENT_ID` (`wallet-service`), porque a seção 2 reserva operação de carteira ao serviço interno. `/health/live` e `/health/ready` não exigem token: a seção 9 pede health público. `POST /wagering/transactions` recusa o cliente interno e recusa `providerId` diferente do `azp`, então um token não aposta em nome de outro provedor. A leitura da transação de outro provedor responde `404` `transaction_not_found`, o mesmo código de uma transação inexistente, para não confirmar que a transação existe. O replay segue a transação gravada e continua sujeito a essa leitura.

O LocalStack não tem IAM. O processo usa as credenciais estáticas `test` / `test` só no `SQS_ENDPOINT` e não lê a cadeia da máquina. Criar e usar as filas exige esse par. Isso não substitui a regra de domínio: o consumidor, depois de receber a mensagem, ainda valida chave, saldo, jogador e referência. Não há uma política de fila por provedor. Quem consegue publicar no endpoint local pode enfileirar um corpo; quem decide se o dinheiro anda é o caso de uso.

## Fx

`fx.New` constrói o pool, a store, o verificador OIDC, as filas, os serviços de aplicação e o handler HTTP por `fx.Provide`. Não há `fx.Module`: o processo é um só, e a lista de construtores mora em `cmd/api`. `fx.Invoke(serve)` sobe o HTTP e os três workers. `OnStop` cancela os workers, espera as três goroutines saírem, encerra o HTTP e só então fecha o pool. O pool tenta a primeira conexão por até 30 segundos, para a API subir enquanto o PostgreSQL ainda fica pronto.

## Observabilidade

Os logs são JSON na stdout. O log de uma aposta inclui `correlationId`, `messageId`, `transactionId`, `walletId`, `providerId` e `status`. Não inclui o valor em dinheiro, um token nem o payload completo.

O `expvar` publica `wager_results`, `wager_duplicates`, `wager_retries`, `wager_version_conflicts`, `wager_dlq`, `outbox_retries`, `outbox_lag_ms`, `wager_processing_ms` e `reconciliation_divergences`. Uma diferença de reconciliação diferente de zero também escreve um aviso com o id da carteira, a diferença, a moeda e a quantidade de lançamentos.

## Erros HTTP

| Situação | Status | `error` ou estado | `failureCode` gravado |
| --- | --- | --- | --- |
| Token ausente ou inválido | 401 | `unauthorized` | nenhum |
| Provedor numa rota de carteira, cliente interno apostando, ou `providerId` diferente do `azp` | 403 | `forbidden` | nenhum |
| Chave de idempotência ausente ou corpo malformado | 400 | `idempotency_key_required` ou `invalid_body` | nenhum |
| `kind` que o caso de uso não aceita, inclusive `OPENING` | 400 | `unsupported_kind` | nenhum |
| Carteira desconhecida | 404 | `wallet_not_found` | nenhum |
| Transação desconhecida, inclusive a de outro provedor | 404 | `transaction_not_found` | nenhum |
| Jogador e moeda repetidos | 409 | `wallet_already_exists` | nenhum |
| Chave ou id externo reutilizado com outro conteúdo | 409 | `idempotency_conflict` | nenhum |
| Jogador do corpo diferente do dono da carteira | 422 | `bet_player_mismatch` | nenhum |
| Aposta maior que o saldo | 422 | `insufficient_funds` | `INSUFFICIENT_FUNDS` |
| Referência incompatível ou valor de reversão diferente | 422 | `invalid_reference` | nenhum na chamada imediata; o worker pode gravar `INVALID_REFERENCE` numa espera que ficou incompatível |
| Segunda reversão processada da mesma referência | 422 | `duplicate_reversal` | `DUPLICATE_REVERSAL` |
| Reversão que debitaria além do saldo | 422 | `reversal_insufficient_funds` | `REVERSAL_INSUFFICIENT_FUNDS` |
| Espera da referência esgotada, no replay | 422 | `reference_not_found` | `REFERENCE_NOT_FOUND` |
| Referência ainda ausente ou pendente | 202 | `PENDING_REFERENCE` | nenhum |
| Aposta processada | 200 | `PROCESSED` | nenhum |
| Versão esgotada, valor inválido ou moeda diferente da carteira | 500 | `place_bet_failed` | nenhum |

Uma linha `REJECTED` é definitiva para aquela chave: o replay devolve o mesmo `422` e não debita. Corrigir é outra operação, com outra chave e outro id externo. Por exemplo, `80.00` sobre `75.00` fica `INSUFFICIENT_FUNDS`; uma aposta nova de `1.00` pode passar. `idempotency_conflict` e `bet_player_mismatch` não gravam linha. A chave original continua presa ao primeiro corpo. Valor `0.00` numa aposta, `LOSS` diferente de `0.00` e moeda diferente da carteira não têm `failureCode`: o domínio recusa antes de gravar, e o HTTP cai em `place_bet_failed`.

## Limitações

- Tracing com OpenTelemetry, dashboards, partidas dobradas e testes de carga são opcionais no enunciado e não foram implementados.
- `outbox_lag_ms` e `wager_processing_ms` guardam a última observação, não um histograma. `/metrics` também inclui `cmdline` e `memstats`, que o `expvar` publica para o processo.
- Cinquenta processos enviam a mesma aposta de `25.00` numa carteira de `100.00` e deixam um débito em `75.00`. Dois processos disputam duas apostas de `80.00` sobre `100.00` e só uma passa. Três processos apostam `80.00` em três carteiras de `100.00` e cada uma termina em `20.00`.
- Uma mensagem cujo delete falha depois do commit é reentregue e não debita de novo. Um segundo processo resolve um `PENDING_REFERENCE` gravado por um processo que já parou.
- O Compose não declara três réplicas da API. As instâncias independentes dos testes são processos separados, e uma segunda API sobe com outro `HTTP_ADDR`.
- As migrations de descida são arquivos SQL. O processo aplica só `*.up.sql`.
- A nova tentativa por conflito de versão ocorre no máximo 3 vezes dentro de um pedido. Conflitos além disso aparecem como falha de persistência, sem linha `FAILED`. O predicado no banco continua impedindo uma atualização perdida.
- A composição Fx não usa `fx.Module`. Há uma lista de `fx.Provide` e um `fx.Invoke` em `cmd/api`.
- A inbox é gravada depois do commit da aposta, não dentro dele. A chave de idempotência cobre esse intervalo.
