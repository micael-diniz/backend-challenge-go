# Arquitetura

Os requisitos em [CHALLENGE.md](CHALLENGE.md) foram a origem de cada comportamento. Um teste descrevia o resultado exigido antes do código de produção. Esse código entrava só até o teste passar. As regras de domínio foram provadas antes de existir um repositório. O PostgreSQL então provou as mesmas regras numa transação real, com constraint e com reinício. O adaptador em memória permanece para os testes unitários de HTTP. Ele não é a evidência de durabilidade, de idempotência entre processos nem de recuperação.

## Domínio e repositórios

`internal/domain` guarda `Money`, `Wallet`, `WagerTransaction`, `WalletLedgerEntry` e os quatro tipos de evento. Não importa repositório, HTTP nem driver de banco. Os métodos devolvem erros sentinela. Não escrevem log.

`internal/application` guarda os casos de uso e as portas de repositório: abertura de carteira, aposta, leituras, inbox e outbox. O domínio não depende dessas interfaces. `internal/infrastructure/postgres` implementa as portas com `pgx` e com o SQL escrito na store. Não há ORM nem framework de migration. Um ORM esconderia o predicado do `UPDATE` da carteira e a transação única que grava a carteira, a aposta, o ledger e a outbox.

O Fx liga uma store PostgreSQL a cada porta de que o processo precisa. O HTTP fica em `internal/interfaces/httpapi`. O cliente SQS fica em `internal/infrastructure/sqs`. O loop do consumidor, a varredura de referências pendentes e a varredura da outbox ficam em `cmd/api`.

## Dinheiro

Os valores são `int64` com sinal, em unidades menores. `100.00 BRL` é gravado como `10000`. Parse, conta, JSON e SQL não usam `float32` nem `float64`. Um valor externo precisa de exatamente duas casas decimais. `-0.00` vira `0.00`. `BRL` e `USD` são aceitos e gravados em maiúsculas. A representação com sinal existe para a diferença da reconciliação poder ser negativa. Quem recusa saldo negativo é a regra da carteira, não o próprio `Money`.

## Transações

Uma abertura positiva confirma, na mesma transação, a carteira na versão `1`, um `OPENING` processado, um crédito no ledger e as linhas de outbox `WagerTransactionProcessed` e `WalletBalanceChanged`. Uma abertura com saldo zero confirma só a carteira.

Uma aposta síncrona carrega a carteira, monta a aposta pendente, aplica a transição de domínio e confirma juntos a nova versão da carteira, a aposta, o lançamento do ledger e as linhas da outbox. Uma falha faz rollback dessa transação. `LOSS` grava a aposta processada e não muda o saldo. `WIN` credita a aposta referenciada. `REFUND` e `ROLLBACK` creditam ou debitam o movimento referenciado no mesmo valor. Uma segunda reversão dessa referência é `duplicate_reversal`.

O ledger confere a equação de crédito e débito no banco e recusa `UPDATE` e `DELETE` com SQLSTATE `55000`.

## Idempotência

O header HTTP `Idempotency-Key` é gravado como veio. O hash do payload é SHA-256 do JSON canônico de `encoding/json`, com as chaves do objeto ordenadas. Os campos no hash são `providerId`, `externalTransactionId`, `playerId`, `walletId`, `roundId`, `gameId`, `kind` e `money`. `WIN`, `REFUND` e `ROLLBACK` também incluem `referenceExternalTransactionId`. A chave de idempotência e os metadados de transporte ficam de fora. HTTP e SQS usam esse mesmo hash.

A mesma chave e o mesmo hash devolvem o resultado gravado, inclusive o saldo original. A mesma chave com outro hash é conflito. O mesmo `(providerId, externalTransactionId)` com outra chave também é conflito.

O SQS acrescenta o `messageId` do envelope como identidade da inbox. O mesmo id de mensagem e o mesmo hash não chamam de novo o caminho do dinheiro. O mesmo id com outro hash permanece na fila e não é apagado. A linha da inbox é escrita depois do commit da aposta. Se o processo parar entre essas duas escritas, a chave de idempotência já impede um segundo movimento, e a entrega seguinte grava a inbox.

## Locks e concorrência

A linha da carteira é atualizada com `WHERE id = $1 AND version = $2`. Zero linhas atualizadas são conflito de versão. A aplicação tenta de novo esse conflito até 3 vezes no mesmo processo. Dois processos apostando `80.00` sobre `100.00` produzem um débito e uma rejeição por saldo insuficiente. A rejeição fica gravada.

O worker da outbox trava uma linha devida com `FOR UPDATE SKIP LOCKED`. Uma reserva que sofre rollback deixa a linha para outro publicador. Uma confirmação de publicação perdida reenvia o mesmo id de evento. Uma linha com `published_at` preenchido não é enviada de novo.

A reconciliação lê a carteira e o ledger numa transação `REPEATABLE READ, READ ONLY`. O saldo calculado começa em zero, soma créditos e subtrai débitos. `difference` é o saldo armazenado menos o calculado. O saldo armazenado não é alterado.

## Referências pendentes

Um `WIN`, `REFUND` ou `ROLLBACK` cuja referência não existe ou ainda não foi processada fica `PENDING_REFERENCE`. A carteira não se move. A primeira nova tentativa é um segundo depois. Cada espera seguinte dobra, e a espera expira um minuto depois do pedido. No prazo, a linha vira `REJECTED` com `REFERENCE_NOT_FOUND`. Uma referência já rejeitada, falha ou incompatível responde `422` na hora e não grava linha nova.

O agendamento está no PostgreSQL. A API consulta as linhas devidas a cada segundo, então um processo novo continua uma espera que um processo anterior gravou.

## Inbox, outbox e encerramento

Rejeições de negócio já confirmadas são terminais, e o consumidor apaga essas mensagens. Um erro de banco deixa a mensagem para outra tentativa. Um envelope inválido não é apagado. Depois de 5 recebimentos, a fila o reenvia para `wager-transactions-dlq.fifo`.

A publicação da outbox é um worker separado. Ela corre depois do commit da transação de origem. Um erro de publicação aumenta as tentativas e espera 1s, 2s, 4s, no máximo um minuto.

`SIGTERM` cancela o contexto dos workers e depois encerra o servidor HTTP. O consumidor para de pedir mensagens. Uma mensagem já recebida e não apagada volta a ficar visível depois de 30 segundos. Um commit concluído antes do delete fica protegido pela chave de idempotência e, na entrega seguinte, pela inbox.

Os eventos exigidos são tipos concretos: `WagerTransactionProcessed`, `WagerTransactionRejected`, `WalletBalanceChanged` e `WagerTransactionPendingReference`. Os construtores definem o tipo e a versão do evento. Os payloads usam timestamp UTC em RFC 3339 e dinheiro em string decimal.

## Autenticação e autorização

O Keycloak emite access tokens com o grant `client_credentials`. A API verifica assinatura, issuer e expiração com `github.com/coreos/go-oidc/v3`. Ela não cadastra senha e não assina token. O chamador é o claim `azp`. Os tokens deste realm usam `aud=account`, então a checagem de audience fica de fora.

`/health/live` e `/health/ready` não exigem token. Rotas de carteira, reconciliação e `/metrics` exigem `WALLET_CLIENT_ID`. Um provedor só envia aposta quando `azp` é igual ao `providerId` do corpo. O cliente interno não pode apostar. A leitura, por um provedor, da transação de outro provedor responde `404`.

O LocalStack usa as credenciais estáticas `test` / `test` em `SQS_ENDPOINT`. O cliente não lê a cadeia de credenciais AWS da máquina.

## Fx

`fx.New` constrói o pool, a store, o verificador OIDC, as filas, os serviços de aplicação e o handler HTTP. `fx.Invoke(serve)` sobe o HTTP e os três workers. `OnStop` fecha o pool, cancela os workers e encerra o HTTP. O pool tenta a primeira conexão por até 30 segundos, para a API subir enquanto o PostgreSQL ainda fica pronto.

## Observabilidade

Os logs são JSON na stdout. O log de uma aposta inclui `correlationId`, `messageId`, `transactionId`, `walletId`, `providerId` e `status`. Não inclui o valor em dinheiro, um token nem o payload completo.

O `expvar` publica `wager_results`, `wager_duplicates`, `wager_retries`, `wager_version_conflicts`, `wager_dlq`, `outbox_retries`, `outbox_lag_ms`, `wager_processing_ms` e `reconciliation_divergences`. Uma diferença de reconciliação diferente de zero também escreve um aviso com o id da carteira, a diferença, a moeda e a quantidade de lançamentos.

## Erros HTTP

| Situação | Status | `error` |
| --- | --- | --- |
| Token ausente ou inválido | 401 | `unauthorized` |
| Provedor numa rota de carteira, ou cliente interno apostando | 403 | `forbidden` |
| Chave de idempotência ausente ou corpo malformado | 400 | `idempotency_key_required` ou `invalid_body` |
| Carteira desconhecida | 404 | `wallet_not_found` |
| Transação desconhecida, inclusive a de outro provedor | 404 | `transaction_not_found` |
| Jogador e moeda repetidos | 409 | `wallet_already_exists` |
| Chave de idempotência ou id externo reutilizado com outro conteúdo | 409 | `idempotency_conflict` |
| Saldo insuficiente, referência inválida ou reversão duplicada | 422 | código estável da falha |
| Referência ainda não processada | 202 | status do corpo `PENDING_REFERENCE` |
| Aposta processada | 200 | status do corpo `PROCESSED` |

## Limitações

- Tracing com OpenTelemetry, dashboards, partidas dobradas e testes de carga são opcionais no enunciado e não foram implementados.
- `outbox_lag_ms` e `wager_processing_ms` guardam a última observação, não um histograma. `/metrics` também inclui `cmdline` e `memstats`, que o `expvar` publica para o processo.
- Cinquenta processos enviam a mesma aposta de `25.00` numa carteira de `100.00` e deixam um débito em `75.00`. Dois processos disputam duas apostas de `80.00` sobre `100.00` e só uma passa. Três processos apostam `80.00` em três carteiras de `100.00` e cada uma termina em `20.00`.
- Uma mensagem cujo delete falha depois do commit é reentregue e não debita de novo. Um segundo processo resolve um `PENDING_REFERENCE` gravado por um processo que já parou.
- Não há teste de integração em que uma mensagem inválida chegue à DLQ depois de 5 recebimentos. A fila está configurada com esse redrive.
- Não há teste que suba e encerre a aplicação Fx e confira a liberação dos workers. O `OnStop` cancela os workers, encerra o HTTP e fecha o pool.
- O Compose não declara três réplicas da API. As instâncias independentes dos testes são processos separados, e uma segunda API sobe com outro `HTTP_ADDR`.
- As migrations de descida são arquivos SQL. O processo aplica só `*.up.sql`.
- A nova tentativa por conflito de versão ocorre no máximo 3 vezes dentro de um pedido. Conflitos além disso aparecem como falha de persistência. O predicado no banco continua impedindo uma atualização perdida.
