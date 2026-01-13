-- Test schema for doodle with temporal versioning

-- Users table with temporal versioning
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id TEXT NOT NULL,
    email TEXT,
    first_name TEXT,
    last_name TEXT,
    status TEXT DEFAULT 'ACTIVE',
    provider TEXT NOT NULL,

    -- Full API response
    raw_data JSONB,

    -- Temporal versioning
    version INT NOT NULL DEFAULT 1,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to TIMESTAMPTZ NOT NULL DEFAULT 'infinity',

    -- Constraints
    UNIQUE(external_id, version)
);

-- Groups table with temporal versioning
CREATE TABLE groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    provider TEXT NOT NULL,

    raw_data JSONB,
    version INT NOT NULL DEFAULT 1,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to TIMESTAMPTZ NOT NULL DEFAULT 'infinity',

    UNIQUE(external_id, version)
);

-- Apps table with temporal versioning
CREATE TABLE apps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id TEXT NOT NULL,
    name TEXT NOT NULL,
    app_type TEXT,
    provider TEXT NOT NULL,

    raw_data JSONB,
    version INT NOT NULL DEFAULT 1,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to TIMESTAMPTZ NOT NULL DEFAULT 'infinity',

    UNIQUE(external_id, version)
);

-- Junction: user_groups (member_of relationship) with temporal versioning
CREATE TABLE user_groups (
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    group_id UUID REFERENCES groups(id) ON DELETE CASCADE,
    role TEXT DEFAULT 'member',

    -- Temporal versioning
    valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to TIMESTAMPTZ NOT NULL DEFAULT 'infinity',

    PRIMARY KEY (user_id, group_id, valid_from)
);

-- Junction: group_apps (has_access relationship) with temporal versioning
CREATE TABLE group_apps (
    group_id UUID REFERENCES groups(id) ON DELETE CASCADE,
    app_id UUID REFERENCES apps(id) ON DELETE CASCADE,
    permission TEXT DEFAULT 'read',

    -- Temporal versioning
    valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to TIMESTAMPTZ NOT NULL DEFAULT 'infinity',

    PRIMARY KEY (group_id, app_id, valid_from)
);

-- Insert test data (current versions)
INSERT INTO users (external_id, email, first_name, last_name, status, provider, raw_data) VALUES
    ('okta_user_001', 'alice@example.com', 'Alice', 'Smith', 'ACTIVE', 'okta', '{"id": "okta_user_001", "profile": {"email": "alice@example.com"}}'),
    ('okta_user_002', 'bob@example.com', 'Bob', 'Jones', 'ACTIVE', 'okta', '{"id": "okta_user_002", "profile": {"email": "bob@example.com"}}'),
    ('okta_user_003', 'charlie@example.com', 'Charlie', 'Brown', 'SUSPENDED', 'okta', '{"id": "okta_user_003", "profile": {"email": "charlie@example.com"}}'),
    ('azure_user_001', 'diana@example.com', 'Diana', 'Prince', 'ACTIVE', 'azure', '{"id": "azure_user_001", "mail": "diana@example.com"}');

INSERT INTO groups (external_id, name, description, provider, raw_data) VALUES
    ('okta_group_admins', 'Administrators', 'System administrators', 'okta', '{"id": "okta_group_admins", "profile": {"name": "Administrators"}}'),
    ('okta_group_devs', 'Developers', 'Development team', 'okta', '{"id": "okta_group_devs", "profile": {"name": "Developers"}}'),
    ('okta_group_users', 'Users', 'Regular users', 'okta', '{"id": "okta_group_users", "profile": {"name": "Users"}}');

INSERT INTO apps (external_id, name, app_type, provider, raw_data) VALUES
    ('okta_app_slack', 'Slack', 'saml', 'okta', '{"id": "okta_app_slack", "label": "Slack"}'),
    ('okta_app_github', 'GitHub', 'oidc', 'okta', '{"id": "okta_app_github", "label": "GitHub"}'),
    ('okta_app_jira', 'Jira', 'saml', 'okta', '{"id": "okta_app_jira", "label": "Jira"}');

-- Relationships
-- Alice is admin and dev
INSERT INTO user_groups (user_id, group_id, role)
SELECT u.id, g.id, 'admin'
FROM users u, groups g
WHERE u.external_id = 'okta_user_001' AND g.external_id = 'okta_group_admins'
  AND u.valid_to = 'infinity' AND g.valid_to = 'infinity';

INSERT INTO user_groups (user_id, group_id, role)
SELECT u.id, g.id, 'member'
FROM users u, groups g
WHERE u.external_id = 'okta_user_001' AND g.external_id = 'okta_group_devs'
  AND u.valid_to = 'infinity' AND g.valid_to = 'infinity';

-- Bob is dev
INSERT INTO user_groups (user_id, group_id, role)
SELECT u.id, g.id, 'member'
FROM users u, groups g
WHERE u.external_id = 'okta_user_002' AND g.external_id = 'okta_group_devs'
  AND u.valid_to = 'infinity' AND g.valid_to = 'infinity';

-- Charlie is just a user
INSERT INTO user_groups (user_id, group_id, role)
SELECT u.id, g.id, 'member'
FROM users u, groups g
WHERE u.external_id = 'okta_user_003' AND g.external_id = 'okta_group_users'
  AND u.valid_to = 'infinity' AND g.valid_to = 'infinity';

-- Admins have access to all apps
INSERT INTO group_apps (group_id, app_id, permission)
SELECT g.id, a.id, 'admin'
FROM groups g, apps a
WHERE g.external_id = 'okta_group_admins'
  AND g.valid_to = 'infinity' AND a.valid_to = 'infinity';

-- Devs have access to GitHub and Slack
INSERT INTO group_apps (group_id, app_id, permission)
SELECT g.id, a.id, 'write'
FROM groups g, apps a
WHERE g.external_id = 'okta_group_devs' AND a.external_id IN ('okta_app_slack', 'okta_app_github')
  AND g.valid_to = 'infinity' AND a.valid_to = 'infinity';

-- Users have access to Slack only
INSERT INTO group_apps (group_id, app_id, permission)
SELECT g.id, a.id, 'read'
FROM groups g, apps a
WHERE g.external_id = 'okta_group_users' AND a.external_id = 'okta_app_slack'
  AND g.valid_to = 'infinity' AND a.valid_to = 'infinity';

-- Create indexes for performance
CREATE INDEX idx_users_external_id ON users(external_id);
CREATE INDEX idx_users_current ON users(external_id) WHERE valid_to = 'infinity';
CREATE INDEX idx_users_provider ON users(provider);
CREATE INDEX idx_users_raw_data ON users USING gin(raw_data);

CREATE INDEX idx_groups_external_id ON groups(external_id);
CREATE INDEX idx_groups_current ON groups(external_id) WHERE valid_to = 'infinity';

CREATE INDEX idx_apps_external_id ON apps(external_id);
CREATE INDEX idx_apps_current ON apps(external_id) WHERE valid_to = 'infinity';

CREATE INDEX idx_user_groups_user_id ON user_groups(user_id);
CREATE INDEX idx_user_groups_group_id ON user_groups(group_id);
CREATE INDEX idx_user_groups_current ON user_groups(user_id, group_id) WHERE valid_to = 'infinity';
CREATE INDEX idx_group_apps_group_id ON group_apps(group_id);
CREATE INDEX idx_group_apps_app_id ON group_apps(app_id);
CREATE INDEX idx_group_apps_current ON group_apps(group_id, app_id) WHERE valid_to = 'infinity';

-- Insert historical version for Alice (email changed)
INSERT INTO users (external_id, email, first_name, last_name, status, provider, raw_data, version, valid_from, valid_to)
VALUES (
    'okta_user_001',
    'alice.old@example.com',
    'Alice',
    'Smith',
    'ACTIVE',
    'okta',
    '{"id": "okta_user_001", "profile": {"email": "alice.old@example.com"}}',
    0,
    '2024-01-01 00:00:00+00',
    '2024-06-01 00:00:00+00'
);

-- Insert historical relationship: Bob was previously in admins group (removed on 2024-06-01)
INSERT INTO user_groups (user_id, group_id, role, valid_from, valid_to)
SELECT u.id, g.id, 'admin', '2024-01-01 00:00:00+00', '2024-06-01 00:00:00+00'
FROM users u, groups g
WHERE u.external_id = 'okta_user_002' AND g.external_id = 'okta_group_admins'
  AND u.valid_to = 'infinity' AND g.valid_to = 'infinity';
