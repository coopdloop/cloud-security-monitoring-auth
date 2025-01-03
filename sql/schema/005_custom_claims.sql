-- sql/schema/005_custom_claims.sql

-- Create a custom_claims table
CREATE TABLE IF NOT EXISTS custom_claims (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    claim_key VARCHAR(255) NOT NULL,
    claim_value JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, claim_key)
);
