-- T14 core/gitcredential. Forward-only. PAT terenkripsi AES-256-GCM (nonce||ciphertext||tag).
-- user_id FK CASCADE (README konvensi: FK lintas module hanya project_id/user_id + CASCADE):
-- user yang dihapus tidak meninggalkan PAT yatim (D11).
CREATE TABLE IF NOT EXISTS git_credentials (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider    text        NOT NULL,
    host        text        NOT NULL,
    token_enc   bytea       NOT NULL,
    last4       text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT git_credentials_provider_check CHECK (provider IN ('github','gitlab')),
    CONSTRAINT git_credentials_host_check CHECK (host = lower(host) AND char_length(host) BETWEEN 1 AND 253),
    CONSTRAINT git_credentials_github_host CHECK (provider <> 'github' OR host = 'github.com'),
    CONSTRAINT git_credentials_token_enc_len CHECK (octet_length(token_enc) > 28),
    CONSTRAINT git_credentials_last4_len CHECK (char_length(last4) BETWEEN 1 AND 4),
    CONSTRAINT git_credentials_user_provider_host_key UNIQUE (user_id, provider, host)
);
