-- Migration 000007: multiple assignees, subtasks, task comments

-- ─── task_assignees (many-to-many tasks <-> users) ──────────────────
CREATE TABLE IF NOT EXISTS task_assignees (
    task_id     INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    assigned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (task_id, user_id)
);

-- "Which tasks is this user assigned to?"
CREATE INDEX IF NOT EXISTS idx_task_assignees_user ON task_assignees (user_id);

-- Backfill from the old single-assignee column (kept for now)
INSERT INTO task_assignees (task_id, user_id)
SELECT id, assignee_id FROM tasks WHERE assignee_id IS NOT NULL
ON CONFLICT (task_id, user_id) DO NOTHING;

-- ─── subtasks (the "2/8" progress on a card) ────────────────────────
CREATE TABLE IF NOT EXISTS subtasks (
    id         SERIAL PRIMARY KEY,
    task_id    INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    done       BOOLEAN NOT NULL DEFAULT FALSE,
    position   INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_subtasks_task ON subtasks (task_id);

-- ─── task_comments (the comment count on a card) ────────────────────
CREATE TABLE IF NOT EXISTS task_comments (
    id         SERIAL PRIMARY KEY,
    task_id    INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    user_id    INTEGER REFERENCES users(id) ON DELETE SET NULL, -- keep comments if the author is deleted
    body       TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_task_comments_task ON task_comments (task_id);