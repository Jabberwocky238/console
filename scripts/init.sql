-- Control Plane Database Schema

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    uid VARCHAR(64) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    secret_key VARCHAR(256) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Verification codes table
CREATE TABLE IF NOT EXISTS verification_codes (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    code VARCHAR(6) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    used BOOLEAN DEFAULT false
);

-- Custom domains table
CREATE TABLE IF NOT EXISTS custom_domains (
    id SERIAL PRIMARY KEY,
    cdid VARCHAR(64) UNIQUE NOT NULL,
    user_uid VARCHAR(64) NOT NULL,
    domain VARCHAR(255) UNIQUE NOT NULL,
    target VARCHAR(255) NOT NULL,
    txt_name VARCHAR(255) NOT NULL,
    txt_value VARCHAR(255) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Workers table
CREATE TABLE IF NOT EXISTS workers (
    id SERIAL PRIMARY KEY,
    wid VARCHAR(64) UNIQUE NOT NULL,
    user_uid VARCHAR(64) NOT NULL,
    worker_name VARCHAR(255) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'unloaded',
    active_version_id INTEGER,
    env_json TEXT NOT NULL DEFAULT '{}',
    secrets_json TEXT NOT NULL DEFAULT '[]',
    assigned_cpu VARCHAR(32) NOT NULL DEFAULT '1',
    assigned_memory VARCHAR(32) NOT NULL DEFAULT '500Mi',
    assigned_disk VARCHAR(32) NOT NULL DEFAULT '2Gi',
    max_replicas INTEGER NOT NULL DEFAULT 1,
    main_region VARCHAR(64) NOT NULL DEFAULT 'global',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_workers_user_uid ON workers(user_uid);
CREATE INDEX IF NOT EXISTS idx_workers_wid ON workers(wid);

-- Worker deploy versions table
CREATE TABLE IF NOT EXISTS worker_deploy_versions (
    id SERIAL PRIMARY KEY,
    worker_id INTEGER NOT NULL REFERENCES workers(id) ON DELETE CASCADE,
    image VARCHAR(512) NOT NULL,
    port INTEGER NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'loading',
    msg TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_wdv_worker_id ON worker_deploy_versions(worker_id);

-- Combinator resources table
CREATE TABLE IF NOT EXISTS combinator_resources (
    id SERIAL PRIMARY KEY,
    user_uid VARCHAR(64) NOT NULL,
    resource_type VARCHAR(32) NOT NULL,
    resource_id VARCHAR(64) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'loading',
    msg TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_combinator_resources_user_uid ON combinator_resources(user_uid);

-- Combinator KV reports table
CREATE TABLE IF NOT EXISTS combinator_resource_reports (
    id SERIAL PRIMARY KEY,
    user_uid VARCHAR(64) NOT NULL,
    resource_id INTEGER NOT NULL REFERENCES combinator_resources(id) ON DELETE CASCADE,
    datachange INTEGER NOT NULL,
    record_start TIMESTAMP NOT NULL,
    record_end TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_combinator_resource_reports_user_uid ON combinator_resource_reports(user_uid);
CREATE INDEX IF NOT EXISTS idx_combinator_resource_reports_resource_id ON combinator_resource_reports(resource_id);

-- Console tasks table
CREATE TABLE IF NOT EXISTS console_tasks (
    id SERIAL PRIMARY KEY,
    task_type TEXT NOT NULL,
    task_status TEXT NOT NULL,
    task_detailed_status TEXT NOT NULL,
    task_info TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_console_tasks_status ON console_tasks(task_status);
CREATE INDEX IF NOT EXISTS idx_console_tasks_type ON console_tasks(task_type);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_verification_codes_email ON verification_codes(email);
CREATE INDEX IF NOT EXISTS idx_custom_domains_user_uid ON custom_domains(user_uid);
CREATE INDEX IF NOT EXISTS idx_custom_domains_domain ON custom_domains(domain);
