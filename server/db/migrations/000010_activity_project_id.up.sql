ALTER TABLE activity_log
    ADD COLUMN project_id INTEGER REFERENCES projects(id) ON DELETE CASCADE;

-- Backfill history so existing activity shows up in the team feed.
UPDATE activity_log a SET project_id = t.project_id
FROM tasks t WHERE a.entity_type = 'task' AND a.entity_id = t.id;

UPDATE activity_log a SET project_id = p.id
FROM projects p WHERE a.entity_type = 'project' AND a.entity_id = p.id;

CREATE INDEX idx_activity_log_project_created ON activity_log (project_id, created_at DESC);