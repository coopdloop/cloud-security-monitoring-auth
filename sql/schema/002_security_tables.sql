-- sql/schema/002_security_tables.sql

-- Modify users table to add auth0_id
ALTER TABLE users
ADD COLUMN IF NOT EXISTS auth0_id VARCHAR(255) UNIQUE;

-- Add MFA status to existing users table if not exists
ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_enabled BOOLEAN DEFAULT FALSE;

-- Sessions table for tracking active user sessions
CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id INTEGER NOT NULL REFERENCES users(id),
    device VARCHAR(255) NOT NULL,
    location VARCHAR(255),
    ip_address INET NOT NULL,
    user_agent TEXT,
    last_active TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMP WITH TIME ZONE
);

-- Security audit log table
CREATE TABLE IF NOT EXISTS security_logs (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    event_type VARCHAR(50) NOT NULL,
    event_description TEXT NOT NULL,
    ip_address INET NOT NULL,
    user_agent TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_last_active ON sessions(last_active);
CREATE INDEX IF NOT EXISTS idx_security_logs_user_id ON security_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_security_logs_created_at ON security_logs(created_at);

-- Enum for event types
DO $$ BEGIN
    CREATE TYPE security_event_type AS ENUM (
        'login',
        'logout',
        'mfa_enabled',
        'mfa_disabled',
        'password_changed',
        'suspicious_activity',
        'session_revoked'
    );
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;
