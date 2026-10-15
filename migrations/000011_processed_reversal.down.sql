DROP INDEX wager_transactions_processed_reversal_reference_unique;

ALTER TABLE wallet_ledger_entries
    DISABLE TRIGGER wallet_ledger_entries_append_only;

DELETE FROM wallet_ledger_entries
WHERE transaction_id IN (
    SELECT id FROM wager_transactions WHERE kind IN ('REFUND', 'ROLLBACK')
);

ALTER TABLE wallet_ledger_entries
    ENABLE TRIGGER wallet_ledger_entries_append_only;

DELETE FROM wager_transactions
WHERE kind IN ('REFUND', 'ROLLBACK');

ALTER TABLE wager_transactions
    DROP CONSTRAINT wager_transactions_reversal_shape;

ALTER TABLE wager_transactions
    DROP CONSTRAINT wager_transactions_kind_supported;

ALTER TABLE wager_transactions
    ADD CONSTRAINT wager_transactions_kind_supported CHECK (
        kind IN ('OPENING', 'BET', 'LOSS', 'WIN')
    );
