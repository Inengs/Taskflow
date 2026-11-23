-- Demo data for LOCAL development. Run with: -v me_email=<your account email>
\set ON_ERROR_STOP on
SELECT set_config('seed.email', :'me_email', false);

BEGIN;

CREATE FUNCTION pg_temp.add_task(
  p int, creator int, ttl text, descr text, st text, pr text,
  due_days int, pos int, age_h int
) RETURNS int AS $$
  INSERT INTO tasks (project_id, user_id, title, description, status, priority,
                     due_date, position, created_at, updated_at)
  VALUES (p, creator, ttl, descr, st, pr,
          CASE WHEN due_days IS NULL THEN NULL ELSE NOW() + make_interval(days => due_days) END,
          pos, NOW() - make_interval(hours => age_h), NOW() - make_interval(hours => age_h))
  RETURNING id;
$$ LANGUAGE sql;

CREATE FUNCTION pg_temp.add_subtasks(task int, total int, done int) RETURNS void AS $$
  INSERT INTO subtasks (task_id, title, done, position)
  SELECT task, 'Step ' || g, g <= done, g - 1 FROM generate_series(1, total) g;
$$ LANGUAGE sql;

DO $$
DECLARE
  me int;
  ada int; tunde int; chioma int; david int;
  p1 int; p2 int; p3 int; p4 int;
  t int; t_mobile int; t_pay int; t_auth int; t_design int;
BEGIN
  SELECT id INTO me FROM users WHERE lower(email) = lower(current_setting('seed.email'));
  IF me IS NULL THEN
    RAISE EXCEPTION 'No user with email %', current_setting('seed.email');
  END IF;

  -- Remove previous demo data (cascades to members, tasks, comments, invitations, activity).
  DELETE FROM projects WHERE name LIKE '% (demo)';
  DELETE FROM users WHERE email LIKE '%@seed.local';

  -- Teammates. The password is not a valid hash, so nobody can log in as them.
  INSERT INTO users (username, email, password, verified, avatar_url)
  VALUES ('ada', 'ada@seed.local', '!', true, 'https://i.pravatar.cc/150?img=5') RETURNING id INTO ada;
  INSERT INTO users (username, email, password, verified, avatar_url)
  VALUES ('tunde', 'tunde@seed.local', '!', true, 'https://i.pravatar.cc/150?img=12') RETURNING id INTO tunde;
  INSERT INTO users (username, email, password, verified, avatar_url)
  VALUES ('chioma', 'chioma@seed.local', '!', true, 'https://i.pravatar.cc/150?img=47') RETURNING id INTO chioma;
  INSERT INTO users (username, email, password, verified, avatar_url)
  VALUES ('david', 'david@seed.local', '!', true, 'https://i.pravatar.cc/150?img=33') RETURNING id INTO david;

  -- ── Project 1: you are the OWNER ───────────────────────────────────────────
  INSERT INTO projects (name, description, user_id)
  VALUES ('Product Launch 2026 (demo)', 'Monitor all of your tasks here.', me) RETURNING id INTO p1;
  INSERT INTO project_members (project_id, user_id, role) VALUES
    (p1, me, 'owner'), (p1, ada, 'admin'), (p1, tunde, 'member'),
    (p1, chioma, 'member'), (p1, david, 'member');

  -- To Do
  t := pg_temp.add_task(p1, me, 'Homepage Redesign', 'Improve visual hierarchy, navigation flow, and conversion.', 'pending', 'high', 21, 0, 30);
  INSERT INTO task_assignees (task_id, user_id) VALUES (t, ada), (t, tunde), (t, chioma);
  PERFORM pg_temp.add_subtasks(t, 8, 0);
  t := pg_temp.add_task(p1, ada, 'Competitor Research Analysis', 'Review industry competitors and identify feature gaps.', 'pending', 'medium', 18, 1, 28);
  INSERT INTO task_assignees (task_id, user_id) VALUES (t, ada), (t, david);
  PERFORM pg_temp.add_subtasks(t, 6, 0);
  t := pg_temp.add_task(p1, me, 'Multi-Language Support Planning', 'Define localization requirements and translation workflow.', 'pending', 'high', 17, 2, 26);
  INSERT INTO task_assignees (task_id, user_id) VALUES (t, me), (t, tunde);
  t := pg_temp.add_task(p1, me, 'Fix broken signup email', 'Welcome email is not sent for new accounts.', 'pending', 'high', -2, 3, 5);
  INSERT INTO task_assignees (task_id, user_id) VALUES (t, me);

  -- In Progress
  t_mobile := pg_temp.add_task(p1, me, 'Mobile Dashboard Development', 'Building responsive dashboard components for small screens.', 'in-progress', 'high', 15, 0, 60);
  INSERT INTO task_assignees (task_id, user_id) VALUES (t_mobile, me), (t_mobile, david), (t_mobile, ada);
  PERFORM pg_temp.add_subtasks(t_mobile, 8, 2);
  t := pg_temp.add_task(p1, tunde, 'User Onboarding Experience', 'Designing onboarding screens to improve user activation.', 'in-progress', 'low', 16, 1, 55);
  INSERT INTO task_assignees (task_id, user_id) VALUES (t, chioma), (t, tunde);
  PERFORM pg_temp.add_subtasks(t, 8, 5);

  -- In Review
  t_design := pg_temp.add_task(p1, ada, 'Design System Update', 'Reviewing updated components and accessibility fixes.', 'in-review', 'medium', 12, 0, 90);
  INSERT INTO task_assignees (task_id, user_id) VALUES (t_design, ada), (t_design, me);
  PERFORM pg_temp.add_subtasks(t_design, 8, 8);
  t := pg_temp.add_task(p1, david, 'API Documentation', 'Awaiting technical review and approval.', 'in-review', 'low', 11, 1, 88);
  INSERT INTO task_assignees (task_id, user_id) VALUES (t, david), (t, tunde);
  PERFORM pg_temp.add_subtasks(t, 8, 8);
  t := pg_temp.add_task(p1, me, 'Security Compliance Audit', 'Final verification of security standards and requirements.', 'in-review', 'high', 12, 2, 86);
  INSERT INTO task_assignees (task_id, user_id) VALUES (t, me), (t, chioma), (t, ada);

  -- Completed
  t_pay := pg_temp.add_task(p1, tunde, 'Payment Gateway Integration', 'Implemented seamless and secure payment processing.', 'done', 'low', -8, 0, 200);
  INSERT INTO task_assignees (task_id, user_id) VALUES (t_pay, tunde), (t_pay, david);
  PERFORM pg_temp.add_subtasks(t_pay, 10, 10);
  t := pg_temp.add_task(p1, ada, 'User Profile Enhancement', 'Added customizable user preferences and profile fields.', 'done', 'medium', -7, 1, 150);
  INSERT INTO task_assignees (task_id, user_id) VALUES (t, ada), (t, chioma);
  PERFORM pg_temp.add_subtasks(t, 6, 6);
  t_auth := pg_temp.add_task(p1, me, 'Authentication Module', 'Successfully launched secure login and account recovery.', 'done', 'high', -8, 2, 120);
  INSERT INTO task_assignees (task_id, user_id) VALUES (t_auth, me), (t_auth, ada);
  PERFORM pg_temp.add_subtasks(t_auth, 8, 8);

  -- A few comments, plus 25 on one task to exercise pagination.
  INSERT INTO task_comments (task_id, user_id, body, created_at) VALUES
    (t_design, ada, 'Updated the color tokens, please review.', NOW() - interval '3 hours'),
    (t_design, me,  'Looks good. Checking contrast on the buttons.', NOW() - interval '2 hours'),
    (t_pay, david,  'Webhooks are verified in staging.', NOW() - interval '1 day');
  INSERT INTO task_comments (task_id, user_id, body, created_at)
  SELECT t_mobile, (ARRAY[me, ada, tunde])[1 + g % 3], 'Comment number ' || g,
         NOW() - make_interval(mins => (30 - g) * 17)
  FROM generate_series(1, 25) g;

  -- Team activity (what teammates did), shown on the dashboard feed.
  INSERT INTO activity_log (user_id, project_id, entity_type, entity_id, action, target, created_at) VALUES
    (tunde, p1, 'task', t_pay,  'completed', 'Payment Gateway Integration', NOW() - interval '2 hours'),
    (ada,   p1, 'task', t_design, 'created', 'Design System Update', NOW() - interval '5 hours'),
    (me,    p1, 'task', t_auth, 'completed', 'Authentication Module', NOW() - interval '1 day');

  -- ── Project 2: you are a plain MEMBER (owned by ada) ───────────────────────
  INSERT INTO projects (name, description, user_id)
  VALUES ('Q3 Roadmap (demo)', 'Planning and tracking for the third quarter.', ada) RETURNING id INTO p2;
  INSERT INTO project_members (project_id, user_id, role) VALUES
    (p2, ada, 'owner'), (p2, me, 'member'), (p2, tunde, 'member');
  t := pg_temp.add_task(p2, ada, 'Define Q3 OKRs', 'Draft objectives and key results.', 'pending', 'medium', 5, 0, 20);
  INSERT INTO task_assignees (task_id, user_id) VALUES (t, me), (t, ada);
  t := pg_temp.add_task(p2, ada, 'Hiring plan', 'Roles and timeline for the next two quarters.', 'in-progress', 'medium', 9, 0, 18);
  INSERT INTO task_assignees (task_id, user_id) VALUES (t, tunde);
  t := pg_temp.add_task(p2, ada, 'Budget approval', 'Signed off by finance.', 'done', 'low', -3, 0, 100);
  INSERT INTO task_assignees (task_id, user_id) VALUES (t, ada);

  -- ── Project 3: you are an ADMIN (owned by tunde) ───────────────────────────
  INSERT INTO projects (name, description, user_id)
  VALUES ('Marketing Campaign (demo)', 'Holiday season campaign planning.', tunde) RETURNING id INTO p3;
  INSERT INTO project_members (project_id, user_id, role) VALUES
    (p3, tunde, 'owner'), (p3, me, 'admin'), (p3, chioma, 'member');
  t := pg_temp.add_task(p3, tunde, 'Holiday campaign brief', 'Audience, channels and budget.', 'pending', 'high', 6, 0, 12);
  INSERT INTO task_assignees (task_id, user_id) VALUES (t, me);
  t := pg_temp.add_task(p3, tunde, 'Social calendar', 'Posting schedule for December.', 'pending', 'low', 10, 1, 10);
  t := pg_temp.add_task(p3, chioma, 'Landing page copy', 'Headline and hero text.', 'in-progress', 'medium', 4, 0, 8);
  INSERT INTO task_assignees (task_id, user_id) VALUES (t, chioma), (t, me);

  -- ── Project 4: you are NOT a member; you have a pending invitation ─────────
  INSERT INTO projects (name, description, user_id)
  VALUES ('Design Guild (demo)', 'Shared design resources.', david) RETURNING id INTO p4;
  INSERT INTO project_members (project_id, user_id, role) VALUES (p4, david, 'owner'), (p4, ada, 'member');

  INSERT INTO project_invitations (project_id, email, role, invited_by) VALUES
    (p4, (SELECT lower(email) FROM users WHERE id = me), 'member', david),
    (p1, 'new.hire@example.com', 'member', me),
    (p1, 'designer@example.com', 'member', ada);

  RAISE NOTICE 'Demo data created for user id %', me;
END $$;

COMMIT;

-- Quick summary
SELECT p.name, pm.role AS my_role, COUNT(DISTINCT t.id) AS tasks
FROM projects p
JOIN project_members pm ON pm.project_id = p.id
JOIN users u ON u.id = pm.user_id AND lower(u.email) = lower(:'me_email')
LEFT JOIN tasks t ON t.project_id = p.id
WHERE p.name LIKE '% (demo)'
GROUP BY p.name, pm.role
ORDER BY p.name;