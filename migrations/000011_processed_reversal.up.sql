ALTER TABLE wager_transactions
    DROP CONSTRAINT wager_transactions_kind_supported;

ALTER TABLE wager_transactions
    ADD CONSTRAINT wager_transactions_kind_supported CHECK (
        kind IN ('OPENING', 'BET', 'LOSS', 'WIN', 'REFUND', 'ROLLBACK')
    );

ALTER TABLE wager_transactions
    ADD CONSTRAINT wager_transactions_reversal_shape CHECK (
        kind NOT IN ('REFUND', 'ROLLBACK')
        OR (
            status = 'PROCESSED'
            AND provider_id IS NOT NULL
            AND btrim(provider_id) <> ''
            AND external_transaction_id IS NOT NULL
            AND btrim(external_transaction_id) <> ''
            AND idempotency_key IS NOT NULL
            AND btrim(idempotency_key) <> ''
            AND payload_hash IS NOT NULL
            AND btrim(payload_hash) <> ''
            AND round_id IS NOT NULL
            AND btrim(round_id) <> ''
            AND game_id IS NOT NULL
            AND btrim(game_id) <> ''
            AND amount_minor > 0
            AND failure_code IS NULL
            AND reference_external_transaction_id IS NOT NULL
            AND btrim(reference_external_transaction_id) <> ''
            AND resolved_reference_transaction_id IS NOT NULL
            AND result_balance_minor IS NOT NULL
            AND result_balance_minor >= 0
            AND result_balance_currency IS NOT NULL
            AND result_balance_currency = currency
            AND reference_attempts = 0
            AND next_reference_attempt_at IS NULL
            AND reference_expires_at IS NULL
            AND updated_at >= created_at
        )
        OR (
            status = 'REJECTED'
            AND provider_id IS NOT NULL
            AND btrim(provider_id) <> ''
            AND external_transaction_id IS NOT NULL
            AND btrim(external_transaction_id) <> ''
            AND idempotency_key IS NOT NULL
            AND btrim(idempotency_key) <> ''
            AND payload_hash IS NOT NULL
            AND btrim(payload_hash) <> ''
            AND round_id IS NOT NULL
            AND btrim(round_id) <> ''
            AND game_id IS NOT NULL
            AND btrim(game_id) <> ''
            AND amount_minor > 0
            AND failure_code IS NOT NULL
            AND failure_code IN (
                'REVERSAL_INSUFFICIENT_FUNDS',
                'DUPLICATE_REVERSAL',
                'INVALID_REFERENCE'
            )
            AND reference_external_transaction_id IS NOT NULL
            AND btrim(reference_external_transaction_id) <> ''
            AND resolved_reference_transaction_id IS NULL
            AND result_balance_minor IS NULL
            AND result_balance_currency IS NULL
            AND reference_attempts = 0
            AND next_reference_attempt_at IS NULL
            AND reference_expires_at IS NULL
            AND updated_at >= created_at
        )
    );

CREATE UNIQUE INDEX wager_transactions_processed_reversal_reference_unique
    ON wager_transactions (provider_id, reference_external_transaction_id)
    WHERE status = 'PROCESSED'
        AND kind IN ('REFUND', 'ROLLBACK');
