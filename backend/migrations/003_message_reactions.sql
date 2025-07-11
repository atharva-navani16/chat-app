CREATE TABLE IF NOT EXISTS message_reactions (
    message_id UUID REFERENCES messages(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    reaction_type VARCHAR(50) NOT NULL, -- emoji unicode or name
    created_at TIMESTAMP DEFAULT NOW(),
    
    PRIMARY KEY (message_id, user_id, reaction_type),
    
    -- Constraints
    CONSTRAINT valid_reaction_type CHECK (
        reaction_type IN ('👍', '👎', '❤️', '😂', '😮', '😢', '😡', '🔥', '👏', '🎉')
    )
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_message_reactions_message ON message_reactions(message_id);
CREATE INDEX IF NOT EXISTS idx_message_reactions_user ON message_reactions(user_id);
CREATE INDEX IF NOT EXISTS idx_message_reactions_type ON message_reactions(reaction_type);

-- Function to get reaction counts for a message
CREATE OR REPLACE FUNCTION get_message_reaction_counts(msg_id UUID)
RETURNS TABLE(reaction_type VARCHAR(50), count BIGINT, users JSONB) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        mr.reaction_type,
        COUNT(*) as count,
        JSONB_AGG(
            JSONB_BUILD_OBJECT(
                'user_id', u.id,
                'username', u.username,
                'first_name', u.first_name
            )
        ) as users
    FROM message_reactions mr
    JOIN users u ON mr.user_id = u.id
    WHERE mr.message_id = msg_id
    GROUP BY mr.reaction_type
    ORDER BY count DESC, mr.reaction_type;
END;
$$ LANGUAGE plpgsql;

COMMENT ON TABLE message_reactions IS 'Stores user reactions to messages (emojis)';