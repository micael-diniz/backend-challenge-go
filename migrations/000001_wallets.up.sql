CREATE TABLE wallets (
    id UUID PRIMARY KEY,
    player_id UUID NOT NULL,
    currency TEXT NOT NULL,
    balance_minor BIGINT NOT NULL,
    version BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT wallets_player_currency_unique UNIQUE (player_id, currency),
    CONSTRAINT wallets_currency_supported CHECK (currency IN ('BRL', 'USD')),
    CONSTRAINT wallets_balance_nonnegative CHECK (balance_minor >= 0),
    CONSTRAINT wallets_version_positive CHECK (version > 0),
    CONSTRAINT wallets_timestamps_ordered CHECK (updated_at >= created_at)
);
