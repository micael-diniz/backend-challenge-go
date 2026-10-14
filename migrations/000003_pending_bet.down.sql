DELETE FROM wager_transactions
WHERE kind = 'BET';

ALTER TABLE wager_transactions
    DROP CONSTRAINT wager_transactions_pending_bet_shape;

ALTER TABLE wager_transactions
    DROP CONSTRAINT wager_transactions_kind_supported;

ALTER TABLE wager_transactions
    ADD CONSTRAINT wager_transactions_kind_supported CHECK (
        kind = 'OPENING'
    );
