-- +goose Up
ALTER TABLE sessions
    ADD COLUMN credential_hash BYTEA NULL UNIQUE,
    ADD COLUMN expires_at TIMESTAMPTZ NULL,
    ADD CONSTRAINT sessions_credential_expiry_check CHECK (
        (credential_hash IS NULL AND expires_at IS NULL) OR
        (credential_hash IS NOT NULL AND expires_at IS NOT NULL
         AND octet_length(credential_hash) = 32));

-- +goose Down
ALTER TABLE sessions
    DROP CONSTRAINT sessions_credential_expiry_check,
    DROP COLUMN expires_at,
    DROP COLUMN credential_hash;
