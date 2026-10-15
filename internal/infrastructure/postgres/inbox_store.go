package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

func (store *WalletOpeningStore) FindInbox(ctx context.Context, messageID string) (application.InboxRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return application.InboxRecord{}, false, err
	}
	var hash string
	var transactionID *string
	err := store.connection.QueryRow(ctx, `
		SELECT payload_hash, transaction_id::text
		FROM inbox_messages
		WHERE message_id = $1`, messageID).Scan(&hash, &transactionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.InboxRecord{}, false, nil
	}
	if err != nil {
		return application.InboxRecord{}, false, fmt.Errorf("find inbox: %w", err)
	}
	record := application.InboxRecord{MessageID: messageID, PayloadHash: hash}
	if transactionID != nil {
		record.TransactionID = domain.TransactionID(*transactionID)
	}
	return record, true, nil
}

func (store *WalletOpeningStore) SaveInbox(ctx context.Context, record application.InboxRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var transactionID any
	if record.TransactionID != "" {
		transactionID = string(record.TransactionID)
	}
	_, err := store.connection.Exec(ctx, `
		INSERT INTO inbox_messages (message_id, payload_hash, transaction_id, received_at)
		VALUES ($1, $2, $3::uuid, $4)`,
		record.MessageID,
		record.PayloadHash,
		transactionID,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("save inbox: %w", err)
	}
	return nil
}
