CREATE FUNCTION wallet_ledger_entries_reject_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'wallet ledger entries are append-only'
        USING ERRCODE = '55000';
END;
$$;

CREATE TABLE wallet_ledger_entries (
    id UUID PRIMARY KEY,
    wallet_id UUID NOT NULL,
    transaction_id UUID NOT NULL,
    direction TEXT NOT NULL,
    amount_minor BIGINT NOT NULL,
    balance_before_minor BIGINT NOT NULL,
    balance_after_minor BIGINT NOT NULL,
    currency TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT wallet_ledger_entries_wallet_fkey
        FOREIGN KEY (wallet_id) REFERENCES wallets (id),
    CONSTRAINT wallet_ledger_entries_transaction_fkey
        FOREIGN KEY (transaction_id) REFERENCES wager_transactions (id),
    CONSTRAINT wallet_ledger_entries_wallet_transaction_unique
        UNIQUE (wallet_id, transaction_id),
    CONSTRAINT wallet_ledger_entries_direction_supported CHECK (
        direction IN ('CREDIT', 'DEBIT')
    ),
    CONSTRAINT wallet_ledger_entries_currency_supported CHECK (
        currency IN ('BRL', 'USD')
    ),
    CONSTRAINT wallet_ledger_entries_amount_positive CHECK (
        amount_minor > 0
    ),
    CONSTRAINT wallet_ledger_entries_balances_nonnegative CHECK (
        balance_before_minor >= 0
        AND balance_after_minor >= 0
    ),
    CONSTRAINT wallet_ledger_entries_equation CHECK (
        direction NOT IN ('CREDIT', 'DEBIT')
        OR (
            direction = 'CREDIT'
            AND balance_after_minor = balance_before_minor + amount_minor
        )
        OR (
            direction = 'DEBIT'
            AND balance_after_minor = balance_before_minor - amount_minor
        )
    )
);

CREATE TRIGGER wallet_ledger_entries_append_only
    BEFORE UPDATE OR DELETE ON wallet_ledger_entries
    FOR EACH ROW
    EXECUTE FUNCTION wallet_ledger_entries_reject_mutation();
