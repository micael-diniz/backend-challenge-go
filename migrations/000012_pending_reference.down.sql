ALTER TABLE wallet_ledger_entries
    DISABLE TRIGGER wallet_ledger_entries_append_only;

DELETE FROM wallet_ledger_entries
WHERE transaction_id IN (
    SELECT id
    FROM wager_transactions
    WHERE kind IN ('WIN', 'REFUND', 'ROLLBACK')
        AND reference_attempts > 0
);

ALTER TABLE wallet_ledger_entries
    ENABLE TRIGGER wallet_ledger_entries_append_only;

DELETE FROM wager_transactions
WHERE kind IN ('WIN', 'REFUND', 'ROLLBACK')
    AND reference_attempts > 0;

ALTER TABLE wager_transactions
    DROP CONSTRAINT wager_transactions_processed_win_shape;

ALTER TABLE wager_transactions
    ADD CONSTRAINT wager_transactions_processed_win_shape CHECK (
        kind <> 'WIN'
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
            AND result_balance_minor IS NOT NULL
            AND result_balance_minor >= 0
            AND result_balance_currency IS NOT NULL
            AND result_balance_currency = currency
            AND (
                (
                    reference_external_transaction_id IS NULL
                    AND resolved_reference_transaction_id IS NULL
                )
                OR (
                    reference_external_transaction_id IS NOT NULL
                    AND btrim(reference_external_transaction_id) <> ''
                    AND resolved_reference_transaction_id IS NOT NULL
                )
            )
            AND reference_attempts = 0
            AND next_reference_attempt_at IS NULL
            AND reference_expires_at IS NULL
            AND updated_at >= created_at
        )
    );

ALTER TABLE wager_transactions
    DROP CONSTRAINT wager_transactions_reversal_shape;

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
