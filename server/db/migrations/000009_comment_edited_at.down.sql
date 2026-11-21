DROP INDEX IF EXISTS idx_subtasks_task_position;
DROP INDEX IF EXISTS idx_task_comments_task_id_id;
ALTER TABLE task_comments DROP COLUMN IF EXISTS edited_at;