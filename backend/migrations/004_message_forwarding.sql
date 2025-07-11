-- migrations/004_message_forwarding.sql
-- Message Forwarding Enhancements

-- Add forwarding tracking table
CREATE TABLE IF NOT EXISTS message_forwards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    original_message_id UUID REFERENCES messages(id) ON DELETE CASCADE,
    forwarded_message_id UUID REFERENCES messages(id) ON DELETE CASCADE,
    forwarded_by UUID REFERENCES users(id),
    forwarded_to_chat_id UUID REFERENCES chats(id),
    forwarded_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(original_message_id, forwarded_message_id)
);

-- Indexes for forwarding
CREATE INDEX IF NOT EXISTS idx_message_forwards_original ON message_forwards(original_message_id);
CREATE INDEX IF NOT EXISTS idx_message_forwards_forwarded ON message_forwards(forwarded_message_id);
CREATE INDEX IF NOT EXISTS idx_message_forwards_user ON message_forwards(forwarded_by);

-- Function to track forward chain depth (prevent infinite forwarding)
CREATE OR REPLACE FUNCTION get_forward_chain_depth(msg_id UUID)
RETURNS INTEGER AS $$
DECLARE
    depth INTEGER := 0;
    current_msg_id UUID := msg_id;
    original_msg_id UUID;
BEGIN
    LOOP
        -- Check if this message is a forward
        SELECT m.forward_from_message_id INTO original_msg_id
        FROM messages m
        WHERE m.id = current_msg_id;
        
        -- If not a forward, break
        IF original_msg_id IS NULL THEN
            EXIT;
        END IF;
        
        depth := depth + 1;
        current_msg_id := original_msg_id;
        
        -- Prevent infinite loops (max depth 10)
        IF depth > 10 THEN
            EXIT;
        END IF;
    END LOOP;
    
    RETURN depth;
END;
$$ LANGUAGE plpgsql;

COMMENT ON TABLE message_forwards IS 'Tracks message forwarding relationships';