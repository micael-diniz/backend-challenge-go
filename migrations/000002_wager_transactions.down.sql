DROP TABLE wager_transactions;

ALTER TABLE wallets
    DROP CONSTRAINT wallets_identity_unique;
