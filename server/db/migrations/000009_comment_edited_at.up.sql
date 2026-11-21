ALTER TABLE task_comments ADD COLUMN edited_at TIMESTAMPTZ;

-- Cursor pagination: WHERE task_id = ? AND id < ? ORDER BY id DESC
CREATE INDEX IF NOT EXISTS idx_task_comments_task_id_id ON task_comments (task_id, id DESC);

-- Subtask lists: WHERE task_id = ? ORDER BY position, id
CREATE INDEX IF NOT EXISTS idx_subtasks_task_position ON subtasks (task_id, position);