CREATE TABLE outbox_events (
    id UUID PRIMARY KEY,
    aggregate_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    attempts INTEGER NOT NULL,
    next_attempt_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,

    CONSTRAINT outbox_events_aggregate_present CHECK (
        length(btrim(aggregate_id)) > 0
    ),
    CONSTRAINT outbox_events_type_supported CHECK (
        event_type IN (
            'WagerTransactionProcessed',
            'WagerTransactionRejected',
            'WalletBalanceChanged',
            'WagerTransactionPendingReference'
        )
    ),
    CONSTRAINT outbox_events_attempts_nonnegative CHECK (
        attempts >= 0
    ),
    CONSTRAINT outbox_events_publication CHECK (
        (published_at IS NULL AND next_attempt_at IS NOT NULL)
        OR (published_at IS NOT NULL AND next_attempt_at IS NULL)
    )
);
