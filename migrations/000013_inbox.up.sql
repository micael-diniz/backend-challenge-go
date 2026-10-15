CREATE TABLE inbox_messages (
    message_id TEXT PRIMARY KEY,
    payload_hash TEXT NOT NULL,
    transaction_id UUID,
    received_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT inbox_messages_id_present CHECK (
        length(btrim(message_id)) > 0
    ),
    CONSTRAINT inbox_messages_hash_present CHECK (
        length(btrim(payload_hash)) > 0
    )
);
