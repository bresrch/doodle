-- Comprehensive seed data for testing all doodle features
-- Run after init.sql to add more test data

-- ============================================================================
-- SCHEMA ADDITIONS (for new features)
-- ============================================================================

-- User managers table for self-referential relationship (variable-length paths)
CREATE TABLE IF NOT EXISTS user_managers (
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    manager_id UUID REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, manager_id)
);

-- Add metadata column with JSONB for JSON function testing
ALTER TABLE users ADD COLUMN IF NOT EXISTS metadata JSONB;
ALTER TABLE users ADD COLUMN IF NOT EXISTS score INTEGER;
ALTER TABLE users ADD COLUMN IF NOT EXISTS salary NUMERIC(10,2);
ALTER TABLE users ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ DEFAULT now();
ALTER TABLE users ADD COLUMN IF NOT EXISTS nickname TEXT;

-- ============================================================================
-- CLEAR EXISTING DATA (for re-runs)
-- ============================================================================

TRUNCATE user_managers CASCADE;
DELETE FROM user_groups;
DELETE FROM group_apps;
DELETE FROM users WHERE external_id LIKE 'seed_%';
DELETE FROM groups WHERE external_id LIKE 'seed_%';
DELETE FROM apps WHERE external_id LIKE 'seed_%';

-- ============================================================================
-- USERS with diverse data for testing
-- ============================================================================

INSERT INTO users (external_id, email, first_name, last_name, status, provider, score, salary, nickname, metadata, created_at) VALUES
    -- Executive team
    ('seed_ceo', 'ceo@acme.com', 'Carol', 'Executive', 'ACTIVE', 'okta', 100, 500000.00, NULL,
     '{"role": "CEO", "department": "Executive", "config": {"theme": "dark", "timezone": "America/New_York"}, "tags": ["executive", "founder"]}',
     '2020-01-15 09:00:00+00'),

    ('seed_cto', 'cto@acme.com', 'Tom', 'Technical', 'ACTIVE', 'okta', 95, 400000.00, 'TechTom',
     '{"role": "CTO", "department": "Engineering", "config": {"theme": "light", "timezone": "America/Los_Angeles"}, "tags": ["executive", "engineering"]}',
     '2020-03-01 10:00:00+00'),

    ('seed_cfo', 'cfo@acme.com', 'Frank', 'Financial', 'ACTIVE', 'okta', 92, 380000.00, NULL,
     '{"role": "CFO", "department": "Finance", "config": {"theme": "dark", "timezone": "America/Chicago"}, "tags": ["executive", "finance"]}',
     '2020-02-20 11:00:00+00'),

    -- Engineering managers
    ('seed_eng_mgr_1', 'eng.mgr1@acme.com', 'Mike', 'Manager', 'ACTIVE', 'okta', 88, 200000.00, 'MikeM',
     '{"role": "Engineering Manager", "department": "Engineering", "config": {"theme": "dark", "timezone": "America/New_York"}, "team_size": 8}',
     '2021-01-10 09:00:00+00'),

    ('seed_eng_mgr_2', 'eng.mgr2@acme.com', 'Maria', 'Lead', 'ACTIVE', 'okta', 85, 190000.00, NULL,
     '{"role": "Engineering Manager", "department": "Engineering", "config": {"theme": "light", "timezone": "Europe/London"}, "team_size": 6}',
     '2021-02-15 10:00:00+00'),

    -- Engineers
    ('seed_eng_1', 'alice.eng@acme.com', 'Alice', 'Engineer', 'ACTIVE', 'okta', 78, 150000.00, 'Ali',
     '{"role": "Senior Engineer", "department": "Engineering", "config": {"theme": "dark", "timezone": "America/New_York"}, "languages": ["Go", "Python"]}',
     '2022-01-05 09:00:00+00'),

    ('seed_eng_2', 'bob.eng@acme.com', 'Bob', 'Builder', 'ACTIVE', 'okta', 72, 140000.00, 'Bobby',
     '{"role": "Engineer", "department": "Engineering", "config": {"theme": "light", "timezone": "America/Los_Angeles"}, "languages": ["JavaScript", "TypeScript"]}',
     '2022-03-20 10:00:00+00'),

    ('seed_eng_3', 'charlie.eng@acme.com', 'Charlie', 'Coder', 'ACTIVE', 'okta', 68, 130000.00, NULL,
     '{"role": "Junior Engineer", "department": "Engineering", "config": {"theme": "dark", "timezone": "America/Chicago"}, "languages": ["Python"]}',
     '2023-01-10 11:00:00+00'),

    ('seed_eng_4', 'diana.eng@acme.com', 'Diana', 'Developer', 'SUSPENDED', 'okta', 45, 120000.00, 'Di',
     '{"role": "Engineer", "department": "Engineering", "config": {"theme": "light", "timezone": "Europe/Berlin"}, "languages": ["Java", "Kotlin"]}',
     '2022-06-15 09:00:00+00'),

    -- Finance team
    ('seed_fin_1', 'frank.fin@acme.com', 'Franklin', 'Accountant', 'ACTIVE', 'okta', 80, 95000.00, NULL,
     '{"role": "Senior Accountant", "department": "Finance", "config": {"theme": "dark", "timezone": "America/New_York"}, "certifications": ["CPA"]}',
     '2021-06-01 09:00:00+00'),

    ('seed_fin_2', 'grace.fin@acme.com', 'Grace', 'Analyst', 'ACTIVE', 'okta', 75, 85000.00, 'Gracie',
     '{"role": "Financial Analyst", "department": "Finance", "config": {"theme": "light", "timezone": "America/New_York"}, "certifications": []}',
     '2022-09-01 10:00:00+00'),

    -- Users with NULL values for testing
    ('seed_null_1', NULL, 'NoEmail', 'User', 'PENDING', 'okta', NULL, NULL, NULL,
     '{"role": "Pending", "department": null}',
     '2023-06-01 09:00:00+00'),

    ('seed_null_2', 'hasmail@acme.com', NULL, NULL, 'ACTIVE', 'okta', 50, 60000.00, '',
     NULL,
     '2023-07-01 10:00:00+00'),

    -- Inactive/deleted users
    ('seed_deleted', 'deleted@acme.com', 'Del', 'Eted', 'DELETED', 'okta', 0, 0, NULL,
     '{"role": "Former Employee", "department": "Unknown"}',
     '2019-01-01 09:00:00+00');

-- ============================================================================
-- GROUPS
-- ============================================================================

INSERT INTO groups (external_id, name, description, provider) VALUES
    ('seed_grp_exec', 'Executive Team', 'C-level executives', 'okta'),
    ('seed_grp_engineering', 'Engineering', 'All engineering staff', 'okta'),
    ('seed_grp_eng_leads', 'Engineering Leads', 'Engineering managers and tech leads', 'okta'),
    ('seed_grp_finance', 'Finance', 'Finance department', 'okta'),
    ('seed_grp_all', 'All Employees', 'All active employees', 'okta'),
    ('seed_grp_empty', 'Empty Group', 'A group with no members', 'okta');

-- ============================================================================
-- APPS
-- ============================================================================

INSERT INTO apps (external_id, name, app_type, provider) VALUES
    ('seed_app_github', 'GitHub Enterprise', 'oidc', 'okta'),
    ('seed_app_slack', 'Slack', 'saml', 'okta'),
    ('seed_app_jira', 'Jira', 'saml', 'okta'),
    ('seed_app_confluence', 'Confluence', 'saml', 'okta'),
    ('seed_app_quickbooks', 'QuickBooks', 'saml', 'okta'),
    ('seed_app_salesforce', 'Salesforce', 'oidc', 'okta'),
    ('seed_app_restricted', 'Restricted App', 'saml', 'okta');

-- ============================================================================
-- MANAGEMENT HIERARCHY (for variable-length path testing)
-- ============================================================================

-- CEO has no manager
-- CTO and CFO report to CEO
INSERT INTO user_managers (user_id, manager_id)
SELECT u.id, m.id FROM users u, users m
WHERE u.external_id = 'seed_cto' AND m.external_id = 'seed_ceo'
  AND u.valid_to = 'infinity' AND m.valid_to = 'infinity';

INSERT INTO user_managers (user_id, manager_id)
SELECT u.id, m.id FROM users u, users m
WHERE u.external_id = 'seed_cfo' AND m.external_id = 'seed_ceo'
  AND u.valid_to = 'infinity' AND m.valid_to = 'infinity';

-- Engineering managers report to CTO
INSERT INTO user_managers (user_id, manager_id)
SELECT u.id, m.id FROM users u, users m
WHERE u.external_id = 'seed_eng_mgr_1' AND m.external_id = 'seed_cto'
  AND u.valid_to = 'infinity' AND m.valid_to = 'infinity';

INSERT INTO user_managers (user_id, manager_id)
SELECT u.id, m.id FROM users u, users m
WHERE u.external_id = 'seed_eng_mgr_2' AND m.external_id = 'seed_cto'
  AND u.valid_to = 'infinity' AND m.valid_to = 'infinity';

-- Engineers report to engineering managers
INSERT INTO user_managers (user_id, manager_id)
SELECT u.id, m.id FROM users u, users m
WHERE u.external_id = 'seed_eng_1' AND m.external_id = 'seed_eng_mgr_1'
  AND u.valid_to = 'infinity' AND m.valid_to = 'infinity';

INSERT INTO user_managers (user_id, manager_id)
SELECT u.id, m.id FROM users u, users m
WHERE u.external_id = 'seed_eng_2' AND m.external_id = 'seed_eng_mgr_1'
  AND u.valid_to = 'infinity' AND m.valid_to = 'infinity';

INSERT INTO user_managers (user_id, manager_id)
SELECT u.id, m.id FROM users u, users m
WHERE u.external_id = 'seed_eng_3' AND m.external_id = 'seed_eng_mgr_2'
  AND u.valid_to = 'infinity' AND m.valid_to = 'infinity';

INSERT INTO user_managers (user_id, manager_id)
SELECT u.id, m.id FROM users u, users m
WHERE u.external_id = 'seed_eng_4' AND m.external_id = 'seed_eng_mgr_2'
  AND u.valid_to = 'infinity' AND m.valid_to = 'infinity';

-- Finance reports to CFO
INSERT INTO user_managers (user_id, manager_id)
SELECT u.id, m.id FROM users u, users m
WHERE u.external_id = 'seed_fin_1' AND m.external_id = 'seed_cfo'
  AND u.valid_to = 'infinity' AND m.valid_to = 'infinity';

INSERT INTO user_managers (user_id, manager_id)
SELECT u.id, m.id FROM users u, users m
WHERE u.external_id = 'seed_fin_2' AND m.external_id = 'seed_fin_1'
  AND u.valid_to = 'infinity' AND m.valid_to = 'infinity';

-- ============================================================================
-- GROUP MEMBERSHIPS (with roles for path field access testing)
-- ============================================================================

-- Executive team
INSERT INTO user_groups (user_id, group_id, role)
SELECT u.id, g.id, 'owner'
FROM users u, groups g
WHERE u.external_id = 'seed_ceo' AND g.external_id = 'seed_grp_exec'
  AND u.valid_to = 'infinity' AND g.valid_to = 'infinity';

INSERT INTO user_groups (user_id, group_id, role)
SELECT u.id, g.id, 'member'
FROM users u, groups g
WHERE u.external_id IN ('seed_cto', 'seed_cfo') AND g.external_id = 'seed_grp_exec'
  AND u.valid_to = 'infinity' AND g.valid_to = 'infinity';

-- Engineering group
INSERT INTO user_groups (user_id, group_id, role)
SELECT u.id, g.id, 'owner'
FROM users u, groups g
WHERE u.external_id = 'seed_cto' AND g.external_id = 'seed_grp_engineering'
  AND u.valid_to = 'infinity' AND g.valid_to = 'infinity';

INSERT INTO user_groups (user_id, group_id, role)
SELECT u.id, g.id, 'admin'
FROM users u, groups g
WHERE u.external_id IN ('seed_eng_mgr_1', 'seed_eng_mgr_2') AND g.external_id = 'seed_grp_engineering'
  AND u.valid_to = 'infinity' AND g.valid_to = 'infinity';

INSERT INTO user_groups (user_id, group_id, role)
SELECT u.id, g.id, 'member'
FROM users u, groups g
WHERE u.external_id IN ('seed_eng_1', 'seed_eng_2', 'seed_eng_3', 'seed_eng_4')
  AND g.external_id = 'seed_grp_engineering'
  AND u.valid_to = 'infinity' AND g.valid_to = 'infinity';

-- Engineering leads group
INSERT INTO user_groups (user_id, group_id, role)
SELECT u.id, g.id, 'member'
FROM users u, groups g
WHERE u.external_id IN ('seed_cto', 'seed_eng_mgr_1', 'seed_eng_mgr_2')
  AND g.external_id = 'seed_grp_eng_leads'
  AND u.valid_to = 'infinity' AND g.valid_to = 'infinity';

-- Finance group
INSERT INTO user_groups (user_id, group_id, role)
SELECT u.id, g.id, 'owner'
FROM users u, groups g
WHERE u.external_id = 'seed_cfo' AND g.external_id = 'seed_grp_finance'
  AND u.valid_to = 'infinity' AND g.valid_to = 'infinity';

INSERT INTO user_groups (user_id, group_id, role)
SELECT u.id, g.id, 'member'
FROM users u, groups g
WHERE u.external_id IN ('seed_fin_1', 'seed_fin_2') AND g.external_id = 'seed_grp_finance'
  AND u.valid_to = 'infinity' AND g.valid_to = 'infinity';

-- All employees group (everyone except deleted)
INSERT INTO user_groups (user_id, group_id, role)
SELECT u.id, g.id, 'member'
FROM users u, groups g
WHERE u.external_id LIKE 'seed_%'
  AND u.external_id != 'seed_deleted'
  AND u.status != 'DELETED'
  AND g.external_id = 'seed_grp_all'
  AND u.valid_to = 'infinity' AND g.valid_to = 'infinity';

-- ============================================================================
-- APP ACCESS (with permissions for path field access testing)
-- ============================================================================

-- Executive: all apps with admin
INSERT INTO group_apps (group_id, app_id, permission)
SELECT g.id, a.id, 'admin'
FROM groups g, apps a
WHERE g.external_id = 'seed_grp_exec' AND a.external_id LIKE 'seed_app_%'
  AND g.valid_to = 'infinity' AND a.valid_to = 'infinity';

-- Engineering: dev tools with write
INSERT INTO group_apps (group_id, app_id, permission)
SELECT g.id, a.id, 'write'
FROM groups g, apps a
WHERE g.external_id = 'seed_grp_engineering'
  AND a.external_id IN ('seed_app_github', 'seed_app_slack', 'seed_app_jira', 'seed_app_confluence')
  AND g.valid_to = 'infinity' AND a.valid_to = 'infinity';

-- Engineering leads: additional admin on dev tools
INSERT INTO group_apps (group_id, app_id, permission)
SELECT g.id, a.id, 'admin'
FROM groups g, apps a
WHERE g.external_id = 'seed_grp_eng_leads'
  AND a.external_id IN ('seed_app_github', 'seed_app_jira')
  AND g.valid_to = 'infinity' AND a.valid_to = 'infinity';

-- Finance: finance apps
INSERT INTO group_apps (group_id, app_id, permission)
SELECT g.id, a.id, 'write'
FROM groups g, apps a
WHERE g.external_id = 'seed_grp_finance'
  AND a.external_id IN ('seed_app_quickbooks', 'seed_app_salesforce', 'seed_app_slack')
  AND g.valid_to = 'infinity' AND a.valid_to = 'infinity';

-- All employees: slack with read
INSERT INTO group_apps (group_id, app_id, permission)
SELECT g.id, a.id, 'read'
FROM groups g, apps a
WHERE g.external_id = 'seed_grp_all' AND a.external_id = 'seed_app_slack'
  AND g.valid_to = 'infinity' AND a.valid_to = 'infinity';

-- ============================================================================
-- INDEXES
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_user_managers_user ON user_managers(user_id);
CREATE INDEX IF NOT EXISTS idx_user_managers_manager ON user_managers(manager_id);
CREATE INDEX IF NOT EXISTS idx_users_metadata ON users USING gin(metadata);
CREATE INDEX IF NOT EXISTS idx_users_score ON users(score);
CREATE INDEX IF NOT EXISTS idx_users_created_at ON users(created_at);

-- ============================================================================
-- EXAMPLE QUERIES TO TEST
-- ============================================================================

-- After loading this seed data, you can test:

-- Basic queries:
--   SELECT * FROM user:seed_eng_1
--   SELECT email, status FROM user WHERE status = 'ACTIVE'

-- Graph traversals:
--   SELECT ->member_of->group FROM user:seed_eng_1
--   SELECT ->member_of->group->has_access->app FROM user:seed_eng_1
--   SELECT <-member_of<-user FROM group:seed_grp_engineering

-- Optional traversals:
--   SELECT ->?member_of->group FROM user:seed_null_1

-- Variable-length paths (management chain):
--   SELECT ->reports_to{1,3}->user FROM user:seed_eng_1
--   SELECT <-reports_to{1,5}<-user FROM user:seed_ceo

-- Path field access (junction table fields):
--   SELECT ->member_of.role->group FROM user:seed_eng_1
--   SELECT ->has_access.permission->app FROM group:seed_grp_engineering

-- Aggregations:
--   SELECT COUNT(*) FROM user WHERE status = 'ACTIVE'
--   SELECT ARRAY_AGG(email) FROM user WHERE status = 'ACTIVE'
--   SELECT STRING_AGG(first_name, ', ') FROM user WHERE status = 'ACTIVE'
--   SELECT JSON_AGG(email) FROM user WHERE status = 'ACTIVE'
--   SELECT status, COUNT(*) FROM user GROUP BY status

-- JSON functions:
--   SELECT JSON_TEXT(metadata, 'role') AS role FROM user:seed_eng_1
--   SELECT JSON_PATH_TEXT(metadata, 'config', 'theme') AS theme FROM user
--   SELECT JSON_BUILD_OBJECT('name', first_name, 'email', email) FROM user

-- Date/time:
--   SELECT created_at + INTERVAL '30 days' AS expires FROM user:seed_eng_1
--   SELECT DATE_TRUNC('month', created_at) AS month FROM user
--   SELECT EXTRACT('year', created_at) AS year FROM user

-- Math functions:
--   SELECT ABS(score - 50) AS diff_from_median FROM user
--   SELECT ROUND(salary / 1000, 0) AS salary_k FROM user

-- String functions:
--   SELECT UPPER(first_name), LOWER(email) FROM user
--   SELECT CONCAT(first_name, ' ', last_name) AS full_name FROM user
--   SELECT LENGTH(email) AS email_length FROM user

-- NULL handling:
--   SELECT * FROM user WHERE email IS NULL
--   SELECT * FROM user WHERE nickname IS NOT NULL
--   SELECT COALESCE(nickname, first_name, 'Unknown') AS display_name FROM user
--   SELECT NULLIF(nickname, '') AS clean_nickname FROM user

-- CASE expressions:
--   SELECT CASE WHEN score > 80 THEN 'High' WHEN score > 50 THEN 'Medium' ELSE 'Low' END AS tier FROM user

-- Set operations:
--   SELECT email FROM user:seed_eng_1 UNION SELECT email FROM user:seed_eng_2
--   SELECT ->member_of->group FROM user:seed_cto INTERSECT SELECT ->member_of->group FROM user:seed_cfo

-- CTEs:
--   WITH active AS (SELECT * FROM user WHERE status = 'ACTIVE') SELECT COUNT(*) FROM active

-- NOT and EXISTS:
--   SELECT * FROM user WHERE NOT status = 'DELETED'
--   SELECT * FROM user WHERE EXISTS (SELECT ->member_of->group FROM user)

-- LIKE:
--   SELECT * FROM user WHERE email LIKE '%@acme.com'
--   SELECT * FROM user WHERE first_name LIKE 'A%'
