CREATE UNIQUE INDEX wager_transactions_provider_external_id_unique
    ON wager_transactions (provider_id, external_transaction_id)
    WHERE provider_id IS NOT NULL;

CREATE UNIQUE INDEX wager_transactions_provider_idempotency_key_unique
    ON wager_transactions (provider_id, idempotency_key)
    WHERE provider_id IS NOT NULL;
