ALTER TABLE wallets
    ADD CONSTRAINT wallets_identity_unique UNIQUE (id, player_id, currency);

CREATE TABLE wager_transactions (
    id UUID PRIMARY KEY,
    provider_id TEXT,
    external_transaction_id TEXT,
    idempotency_key TEXT,
    payload_hash TEXT,
    wallet_id UUID NOT NULL,
    player_id UUID NOT NULL,
    round_id TEXT,
    game_id TEXT,
    kind TEXT NOT NULL,
    amount_minor BIGINT NOT NULL,
    currency TEXT NOT NULL,
    reference_external_transaction_id TEXT,
    resolved_reference_transaction_id UUID,
    status TEXT NOT NULL,
    failure_code TEXT,
    result_balance_minor BIGINT,
    result_balance_currency TEXT,
    reference_attempts INTEGER NOT NULL DEFAULT 0,
    next_reference_attempt_at TIMESTAMPTZ,
    reference_expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT wager_transactions_wallet_identity_fkey
        FOREIGN KEY (wallet_id, player_id, currency)
        REFERENCES wallets (id, player_id, currency),
    CONSTRAINT wager_transactions_kind_supported CHECK (
        kind = 'OPENING'
    ),
    CONSTRAINT wager_transactions_currency_supported CHECK (
        currency IN ('BRL', 'USD')
    ),
    CONSTRAINT wager_transactions_opening_shape CHECK (
        kind <> 'OPENING'
        OR (
            provider_id IS NULL
            AND external_transaction_id IS NULL
            AND idempotency_key IS NULL
            AND payload_hash IS NULL
            AND round_id IS NULL
            AND game_id IS NULL
            AND reference_external_transaction_id IS NULL
            AND resolved_reference_transaction_id IS NULL
            AND status = 'PROCESSED'
            AND amount_minor > 0
            AND failure_code IS NULL
            AND result_balance_minor IS NULL
            AND result_balance_currency IS NULL
            AND reference_attempts = 0
            AND next_reference_attempt_at IS NULL
            AND reference_expires_at IS NULL
            AND updated_at = created_at
        )
    )
);

CREATE UNIQUE INDEX wager_transactions_opening_wallet_unique
    ON wager_transactions (wallet_id)
    WHERE kind = 'OPENING';
