package application

import (
	"expvar"
	"log/slog"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

var (
	wagerResults          = expvar.NewMap("wager_results")
	wagerDuplicates       = expvar.NewInt("wager_duplicates")
	wagerRetries          = expvar.NewInt("wager_retries")
	wagerVersionConflicts = expvar.NewInt("wager_version_conflicts")
	wagerDLQ              = expvar.NewInt("wager_dlq")
	outboxRetries         = expvar.NewInt("outbox_retries")
	outboxLagMilliseconds = expvar.NewInt("outbox_lag_ms")
	wagerProcessingMS     = expvar.NewInt("wager_processing_ms")
)

func RecordWagerResult(status domain.WagerTransactionStatus) {
	if status == "" {
		return
	}
	wagerResults.Add(string(status), 1)
}

func RecordWagerDuplicate() {
	wagerDuplicates.Add(1)
}

func RecordWagerRetry() {
	wagerRetries.Add(1)
	wagerVersionConflicts.Add(1)
}

func RecordWagerDLQ() {
	wagerDLQ.Add(1)
}

func RecordOutboxRetry() {
	outboxRetries.Add(1)
}

func RecordOutboxLag(lag time.Duration) {
	if lag < 0 {
		lag = 0
	}
	outboxLagMilliseconds.Set(lag.Milliseconds())
}

func RecordWagerProcessing(elapsed time.Duration) {
	if elapsed < 0 {
		elapsed = 0
	}
	wagerProcessingMS.Set(elapsed.Milliseconds())
}

func LogWager(command PlaceBetCommand, messageID string, transaction domain.WagerTransaction) {
	slog.Info("wager",
		"correlationId", string(command.CorrelationID),
		"messageId", messageID,
		"transactionId", string(transaction.ID()),
		"walletId", string(command.WalletID),
		"providerId", string(command.ProviderID),
		"status", string(transaction.Status()),
	)
}
