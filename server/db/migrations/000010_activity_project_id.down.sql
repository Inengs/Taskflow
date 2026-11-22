DROP INDEX IF EXISTS idx_activity_log_project_created;
ALTER TABLE activity_log DROP COLUMN IF EXISTS project_id;