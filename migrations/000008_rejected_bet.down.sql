DELETE FROM wager_transactions
WHERE kind = 'BET'
  AND status = 'REJECTED';

ALTER TABLE wager_transactions
    DROP CONSTRAINT wager_transactions_rejected_bet_shape;

ALTER TABLE wager_transactions
    DROP CONSTRAINT wager_transactions_bet_status_supported;

ALTER TABLE wager_transactions
    ADD CONSTRAINT wager_transactions_bet_status_supported CHECK (
        kind <> 'BET'
        OR status IN ('PENDING', 'PROCESSED')
    );
