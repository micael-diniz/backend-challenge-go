DELETE FROM wager_transactions
WHERE kind = 'LOSS';

ALTER TABLE wager_transactions
    DROP CONSTRAINT wager_transactions_processed_loss_shape;

ALTER TABLE wager_transactions
    DROP CONSTRAINT wager_transactions_kind_supported;

ALTER TABLE wager_transactions
    ADD CONSTRAINT wager_transactions_kind_supported CHECK (
        kind IN ('OPENING', 'BET')
    );
