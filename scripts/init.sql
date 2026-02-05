-- Control Plane Database Schema

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    uid VARCHAR(64) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    secret_key VARCHAR(64) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- User RDB resources
CREATE TABLE IF NOT EXISTS user_rdbs (
    id SERIAL PRIMARY KEY,
    uid VARCHAR(64) UNIQUE NOT NULL,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    rdb_type VARCHAR(50) NOT NULL,
    url TEXT NOT NULL,
    enabled BOOLEAN DEFAULT true,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',  -- pending, active, error
    error_msg TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, uid)
);

-- User KV resources
CREATE TABLE IF NOT EXISTS user_kvs (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    uid VARCHAR(64) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    kv_type VARCHAR(50) NOT NULL,
    url TEXT NOT NULL,
    enabled BOOLEAN DEFAULT true,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',  -- pending, active, error
    error_msg TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, uid)
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

-- Workers table
CREATE TABLE IF NOT EXISTS workers (
    id SERIAL PRIMARY KEY,
    worker_id VARCHAR(64) NOT NULL,
    owner_id VARCHAR(64) NOT NULL,
    image TEXT NOT NULL,
    port INTEGER NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    error_msg TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(worker_id, owner_id)
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_user_rdbs_user_id ON user_rdbs(user_id);
CREATE INDEX IF NOT EXISTS idx_user_rdbs_status ON user_rdbs(status);
CREATE INDEX IF NOT EXISTS idx_user_kvs_user_id ON user_kvs(user_id);
CREATE INDEX IF NOT EXISTS idx_user_kvs_status ON user_kvs(status);
CREATE INDEX IF NOT EXISTS idx_verification_codes_email ON verification_codes(email);
