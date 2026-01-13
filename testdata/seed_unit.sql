-- Unit test seed data - extends init.sql without modifying its schema
-- This file adds comprehensive data for unit testing while preserving init.sql data

-- ============================================================================
-- ADDITIONAL USERS (using existing schema from init.sql)
-- ============================================================================

INSERT INTO users (external_id, email, first_name, last_name, status, provider, raw_data) VALUES
    -- Executive team
    ('seed_ceo', 'ceo@acme.com', 'Carol', 'Executive', 'ACTIVE', 'okta',
     '{"role": "CEO", "department": "Executive"}'),
    ('seed_cto', 'cto@acme.com', 'Tom', 'Technical', 'ACTIVE', 'okta',
     '{"role": "CTO", "department": "Engineering"}'),
    ('seed_cfo', 'cfo@acme.com', 'Frank', 'Financial', 'ACTIVE', 'okta',
     '{"role": "CFO", "department": "Finance"}'),

    -- Engineering managers
    ('seed_eng_mgr_1', 'eng.mgr1@acme.com', 'Mike', 'Manager', 'ACTIVE', 'okta',
     '{"role": "Engineering Manager", "department": "Engineering"}'),
    ('seed_eng_mgr_2', 'eng.mgr2@acme.com', 'Maria', 'Lead', 'ACTIVE', 'okta',
     '{"role": "Engineering Manager", "department": "Engineering"}'),

    -- Engineers
    ('seed_eng_1', 'alice.eng@acme.com', 'Alice', 'Engineer', 'ACTIVE', 'okta',
     '{"role": "Senior Engineer", "department": "Engineering"}'),
    ('seed_eng_2', 'bob.eng@acme.com', 'Bob', 'Builder', 'ACTIVE', 'okta',
     '{"role": "Engineer", "department": "Engineering"}'),
    ('seed_eng_3', 'charlie.eng@acme.com', 'Charlie', 'Coder', 'ACTIVE', 'okta',
     '{"role": "Junior Engineer", "department": "Engineering"}'),
    ('seed_eng_4', 'diana.eng@acme.com', 'Diana', 'Developer', 'SUSPENDED', 'okta',
     '{"role": "Engineer", "department": "Engineering"}'),

    -- Finance team
    ('seed_fin_1', 'frank.fin@acme.com', 'Franklin', 'Accountant', 'ACTIVE', 'okta',
     '{"role": "Senior Accountant", "department": "Finance"}'),
    ('seed_fin_2', 'grace.fin@acme.com', 'Grace', 'Analyst', 'ACTIVE', 'okta',
     '{"role": "Financial Analyst", "department": "Finance"}'),

    -- Users with NULL values for testing
    ('seed_null_1', NULL, 'NoEmail', 'User', 'PENDING', 'okta',
     '{"role": "Pending"}'),

    -- Inactive/deleted users
    ('seed_deleted', 'deleted@acme.com', 'Del', 'Eted', 'DELETED', 'okta',
     '{"role": "Former Employee"}');

-- ============================================================================
-- ADDITIONAL GROUPS
-- ============================================================================

INSERT INTO groups (external_id, name, description, provider) VALUES
    ('seed_grp_exec', 'Executive Team', 'C-level executives', 'okta'),
    ('seed_grp_engineering', 'Engineering', 'All engineering staff', 'okta'),
    ('seed_grp_eng_leads', 'Engineering Leads', 'Engineering managers and tech leads', 'okta'),
    ('seed_grp_finance', 'Finance', 'Finance department', 'okta'),
    ('seed_grp_all', 'All Employees', 'All active employees', 'okta'),
    ('seed_grp_empty', 'Empty Group', 'A group with no members', 'okta');

-- ============================================================================
-- ADDITIONAL APPS
-- ============================================================================

INSERT INTO apps (external_id, name, app_type, provider) VALUES
    ('seed_app_github', 'GitHub Enterprise', 'oidc', 'okta'),
    ('seed_app_slack_enterprise', 'Slack Enterprise', 'saml', 'okta'),
    ('seed_app_jira_cloud', 'Jira Cloud', 'saml', 'okta'),
    ('seed_app_confluence', 'Confluence', 'saml', 'okta'),
    ('seed_app_quickbooks', 'QuickBooks', 'saml', 'okta'),
    ('seed_app_salesforce', 'Salesforce', 'oidc', 'okta'),
    ('seed_app_restricted', 'Restricted App', 'saml', 'okta');

-- ============================================================================
-- GROUP MEMBERSHIPS FOR SEED DATA
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

-- All employees group (all seed users except deleted)
INSERT INTO user_groups (user_id, group_id, role)
SELECT u.id, g.id, 'member'
FROM users u, groups g
WHERE u.external_id LIKE 'seed_%'
  AND u.external_id != 'seed_deleted'
  AND u.status != 'DELETED'
  AND g.external_id = 'seed_grp_all'
  AND u.valid_to = 'infinity' AND g.valid_to = 'infinity';

-- ============================================================================
-- APP ACCESS FOR SEED DATA
-- ============================================================================

-- Executive: all seed apps with admin
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
  AND a.external_id IN ('seed_app_github', 'seed_app_slack_enterprise', 'seed_app_jira_cloud', 'seed_app_confluence')
  AND g.valid_to = 'infinity' AND a.valid_to = 'infinity';

-- Engineering leads: admin on dev tools
INSERT INTO group_apps (group_id, app_id, permission)
SELECT g.id, a.id, 'admin'
FROM groups g, apps a
WHERE g.external_id = 'seed_grp_eng_leads'
  AND a.external_id IN ('seed_app_github', 'seed_app_jira_cloud')
  AND g.valid_to = 'infinity' AND a.valid_to = 'infinity';

-- Finance: finance apps
INSERT INTO group_apps (group_id, app_id, permission)
SELECT g.id, a.id, 'write'
FROM groups g, apps a
WHERE g.external_id = 'seed_grp_finance'
  AND a.external_id IN ('seed_app_quickbooks', 'seed_app_salesforce', 'seed_app_slack_enterprise')
  AND g.valid_to = 'infinity' AND a.valid_to = 'infinity';

-- All employees: slack with read
INSERT INTO group_apps (group_id, app_id, permission)
SELECT g.id, a.id, 'read'
FROM groups g, apps a
WHERE g.external_id = 'seed_grp_all' AND a.external_id = 'seed_app_slack_enterprise'
  AND g.valid_to = 'infinity' AND a.valid_to = 'infinity';
