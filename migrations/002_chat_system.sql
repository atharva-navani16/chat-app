-- migrations/002_chat_system.sql
-- Chat System Tables Migration

-- Chats table for storing chat conversations
CREATE TABLE IF NOT EXISTS chats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(20) NOT NULL CHECK (type IN ('private', 'group', 'supergroup', 'channel')),
    title VARCHAR(255),
    description TEXT,
    username VARCHAR(32) UNIQUE, -- For public groups/channels
    
    -- Group/Channel settings
    creator_id UUID REFERENCES users(id),
    member_limit INTEGER DEFAULT 200,
    is_public BOOLEAN DEFAULT false,
    invite_link VARCHAR(100) UNIQUE,
    
    -- Group photo (will be implemented later)
    photo_id UUID,
    
    -- Permissions (JSON for flexibility)
    permissions JSONB DEFAULT '{}',
    
    -- Status
    is_active BOOLEAN DEFAULT true,
    
    -- Metadata
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    CONSTRAINT valid_chat_username CHECK (username ~* '^[a-zA-Z0-9_]{5,32}$')
);

-- Chat members table for tracking user membership in chats
CREATE TABLE IF NOT EXISTS chat_members (
    chat_id UUID REFERENCES chats(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    
    -- Role and permissions
    role VARCHAR(20) DEFAULT 'member' CHECK (role IN ('creator', 'admin', 'member', 'restricted', 'banned')),
    permissions JSONB DEFAULT '{}',
    title VARCHAR(50), -- Custom title for admins
    
    -- Status
    status VARCHAR(20) DEFAULT 'active' CHECK (status IN ('active', 'left', 'kicked', 'banned')),
    can_send_messages BOOLEAN DEFAULT true,
    can_send_media BOOLEAN DEFAULT true,
    can_add_web_page_previews BOOLEAN DEFAULT true,
    
    -- Restrictions
    restricted_until TIMESTAMP,
    
    -- Metadata
    joined_at TIMESTAMP DEFAULT NOW(),
    left_at TIMESTAMP,
    invited_by UUID REFERENCES users(id),
    
    PRIMARY KEY (chat_id, user_id)
);

-- Messages table for storing chat messages
CREATE TABLE IF NOT EXISTS messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id UUID REFERENCES chats(id) ON DELETE CASCADE,
    sender_id UUID REFERENCES users(id),
    
    -- Message content
    message_type VARCHAR(20) DEFAULT 'text' CHECK (message_type IN 
        ('text', 'photo', 'video', 'audio', 'voice', 'document', 'sticker', 
         'location', 'contact', 'poll', 'game', 'service')),
    content TEXT, -- For simple text storage (will be encrypted in production)
    caption TEXT, -- For media messages
    
    -- Encryption (for production use)
    encrypted_content BYTEA,
    nonce BYTEA,
    
    -- Reply and forward
    reply_to_message_id UUID REFERENCES messages(id),
    forward_from_user_id UUID REFERENCES users(id),
    forward_from_chat_id UUID REFERENCES chats(id),
    forward_from_message_id UUID,
    forward_date TIMESTAMP,
    
    -- Media attachments (will be implemented later)
    file_id UUID,
    
    -- Message features
    entities JSONB, -- Text formatting, mentions, hashtags, etc.
    markup JSONB, -- Inline keyboard markup
    
    -- Edit history
    edited_at TIMESTAMP,
    is_edited BOOLEAN DEFAULT false,
    
    -- Status
    is_deleted BOOLEAN DEFAULT false,
    delete_for_everyone BOOLEAN DEFAULT false,
    
    -- Metadata
    created_at TIMESTAMP DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT valid_reply CHECK (reply_to_message_id != id),
    CONSTRAINT valid_content CHECK (
        (content IS NOT NULL AND content != '') OR 
        file_id IS NOT NULL OR 
        message_type != 'text'
    )
);

-- Message delivery status table for read receipts
CREATE TABLE IF NOT EXISTS message_delivery (
    message_id UUID REFERENCES messages(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    
    status VARCHAR(20) DEFAULT 'sent' CHECK (status IN ('sent', 'delivered', 'read')),
    timestamp TIMESTAMP DEFAULT NOW(),
    
    PRIMARY KEY (message_id, user_id)
);

-- Files table for media management (basic structure)
CREATE TABLE IF NOT EXISTS files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- File metadata
    original_name VARCHAR(255),
    file_type VARCHAR(50) NOT NULL,
    mime_type VARCHAR(100),
    file_size BIGINT NOT NULL,
    
    -- Storage information
    storage_path VARCHAR(500) NOT NULL,
    storage_bucket VARCHAR(100),
    cdn_url VARCHAR(500),
    
    -- Media-specific metadata
    width INTEGER,
    height INTEGER,
    duration INTEGER, -- For audio/video in seconds
    thumbnail_file_id UUID REFERENCES files(id),
    
    -- Processing status
    processing_status VARCHAR(20) DEFAULT 'pending' CHECK (processing_status IN 
        ('pending', 'processing', 'completed', 'failed')),
    
    -- Security
    encryption_key BYTEA,
    checksum VARCHAR(64),
    
    -- Access control
    uploaded_by UUID REFERENCES users(id),
    is_public BOOLEAN DEFAULT false,
    
    -- Metadata
    created_at TIMESTAMP DEFAULT NOW(),
    expires_at TIMESTAMP
);

-- User contacts table for contact management
CREATE TABLE IF NOT EXISTS user_contacts (
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    contact_user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    
    -- Contact info
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    phone_number VARCHAR(20),
    
    -- Status
    is_mutual BOOLEAN DEFAULT false,
    is_blocked BOOLEAN DEFAULT false,
    
    -- Metadata
    added_at TIMESTAMP DEFAULT NOW(),
    
    PRIMARY KEY (user_id, contact_user_id),
    CONSTRAINT no_self_contact CHECK (user_id != contact_user_id)
);

-- Create indexes for performance

-- Chat indexes
CREATE INDEX IF NOT EXISTS idx_chats_type ON chats(type);
CREATE INDEX IF NOT EXISTS idx_chats_creator ON chats(creator_id);
CREATE INDEX IF NOT EXISTS idx_chats_username ON chats(username) WHERE username IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_chats_public ON chats(is_public) WHERE is_public = true;
CREATE INDEX IF NOT EXISTS idx_chats_active ON chats(is_active) WHERE is_active = true;

-- Chat members indexes
CREATE INDEX IF NOT EXISTS idx_chat_members_user ON chat_members(user_id);
CREATE INDEX IF NOT EXISTS idx_chat_members_chat ON chat_members(chat_id);
CREATE INDEX IF NOT EXISTS idx_chat_members_role ON chat_members(chat_id, role);
CREATE INDEX IF NOT EXISTS idx_chat_members_status ON chat_members(status);
CREATE INDEX IF NOT EXISTS idx_chat_members_active ON chat_members(chat_id, user_id) WHERE status = 'active';

-- Message indexes (optimized for chat retrieval)
CREATE INDEX IF NOT EXISTS idx_messages_chat_time ON messages(chat_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_messages_sender ON messages(sender_id);
CREATE INDEX IF NOT EXISTS idx_messages_reply ON messages(reply_to_message_id) WHERE reply_to_message_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_messages_type ON messages(message_type);
CREATE INDEX IF NOT EXISTS idx_messages_not_deleted ON messages(chat_id, created_at DESC) WHERE is_deleted = false;

-- Message delivery indexes
CREATE INDEX IF NOT EXISTS idx_delivery_status ON message_delivery(status);
CREATE INDEX IF NOT EXISTS idx_delivery_user_unread ON message_delivery(user_id, status) WHERE status != 'read';

-- File indexes
CREATE INDEX IF NOT EXISTS idx_files_uploader ON files(uploaded_by);
CREATE INDEX IF NOT EXISTS idx_files_type ON files(file_type);
CREATE INDEX IF NOT EXISTS idx_files_created ON files(created_at);

-- Contact indexes
CREATE INDEX IF NOT EXISTS idx_contacts_user ON user_contacts(user_id);
CREATE INDEX IF NOT EXISTS idx_contacts_mutual ON user_contacts(is_mutual) WHERE is_mutual = true;
CREATE INDEX IF NOT EXISTS idx_contacts_blocked ON user_contacts(is_blocked) WHERE is_blocked = true;

-- Functions for updated_at timestamps
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Triggers for automatic updated_at
CREATE TRIGGER update_chats_updated_at BEFORE UPDATE ON chats
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Add some comments for documentation
COMMENT ON TABLE chats IS 'Stores chat conversations (private, group, etc.)';
COMMENT ON TABLE chat_members IS 'Tracks user membership and roles in chats';
COMMENT ON TABLE messages IS 'Stores chat messages with encryption support';
COMMENT ON TABLE message_delivery IS 'Tracks message delivery and read status';
COMMENT ON TABLE files IS 'Stores file metadata for media sharing';
COMMENT ON TABLE user_contacts IS 'Manages user contact relationships';