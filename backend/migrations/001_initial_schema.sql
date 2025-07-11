CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone_number VARCHAR(20) UNIQUE NOT NULL,
    username VARCHAR(32) UNIQUE, -- Telegram-like username (@username)
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100),
    bio TEXT,
    profile_photo_id UUID, -- Reference to files table
    
    -- Privacy settings
    is_public BOOLEAN DEFAULT true, -- Can be found by username
    allow_phone_discovery BOOLEAN DEFAULT true,
    last_seen_privacy VARCHAR(20) DEFAULT 'everyone', -- everyone, contacts, nobody
    
    -- Security
    password_hash VARCHAR(255),
    public_key BYTEA NOT NULL, -- For E2E encryption
    signed_prekey BYTEA NOT NULL,
    prekey_signature BYTEA NOT NULL,
    
    -- Status
    is_online BOOLEAN DEFAULT false,
    last_seen TIMESTAMP,
    status VARCHAR(20) DEFAULT 'active', -- active, deleted, banned
    
    -- Metadata
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- Indexes
    CONSTRAINT valid_username CHECK (username ~* '^[a-zA-Z0-9_]{5,32}$'),
    CONSTRAINT valid_phone CHECK (phone_number ~* '^\+[1-9]\d{1,14}$')
);

-- Indexes for fast lookups
CREATE INDEX idx_users_username ON users(username) WHERE username IS NOT NULL;
CREATE INDEX idx_users_phone ON users(phone_number);
CREATE INDEX idx_users_public ON users(is_public) WHERE is_public = true;