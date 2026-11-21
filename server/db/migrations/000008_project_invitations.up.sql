CREATE TABLE project_invitations (
    id           SERIAL PRIMARY KEY,
    project_id   INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    email        TEXT NOT NULL,
    role         TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member')),
    status       TEXT NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending', 'accepted', 'declined', 'revoked')),
    invited_by   INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '14 days',
    responded_at TIMESTAMPTZ,
    CONSTRAINT project_invitations_email_lower CHECK (email = lower(email))
);

-- One pending invite per (project, email).
CREATE UNIQUE INDEX uq_project_invitations_pending
    ON project_invitations (project_id, email) WHERE status = 'pending';

-- "My invitations" lookup by account email.
CREATE INDEX idx_project_invitations_email_pending
    ON project_invitations (email) WHERE status = 'pending';