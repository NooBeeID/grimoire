CREATE TABLE IF NOT EXISTS user_invitations (
    user_id                  uuid        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    invited_by               uuid        NOT NULL,          -- tanpa FK (pola created_by T01)
    invited_by_label         text        NULL,
    issued_at                timestamptz NOT NULL,          -- terakhir password sementara diterbitkan
    expires_at               timestamptz NOT NULL,
    sent_at                  timestamptz NULL,              -- terakhir BERHASIL terkirim
    delivery_status          text        NOT NULL DEFAULT 'pending'
        CHECK (delivery_status IN ('pending', 'sent', 'failed', 'not_configured')),
    accepted_at              timestamptz NULL,
    resend_count             int         NOT NULL DEFAULT 0 CHECK (resend_count >= 0),
    resend_window_started_at timestamptz NULL,
    resend_window_count      int         NOT NULL DEFAULT 0 CHECK (resend_window_count >= 0),
    created_at               timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS user_invitations_pending_expires_idx
    ON user_invitations (expires_at) WHERE accepted_at IS NULL;

CREATE INDEX IF NOT EXISTS user_invitations_inviter_created_idx
    ON user_invitations (invited_by, created_at);
