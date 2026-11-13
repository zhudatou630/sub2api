-- Calm patch: reserve EVERY pre-existing Anthropic OAuth identity, including
-- soft-deleted accounts. NULL means verified manual import required; it must
-- never be interpreted as permission to generate a replacement ClientID.
CREATE TABLE calm_claude_identities (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    client_id TEXT NULL CHECK (client_id IS NULL OR client_id ~ '^[0-9a-fA-F]{64}$')
);
INSERT INTO calm_claude_identities (account_id, client_id)
SELECT id, NULL FROM accounts WHERE platform = 'anthropic' AND type IN ('oauth', 'setup-token')
ON CONFLICT (account_id) DO NOTHING;

-- Allow verified NULL -> value import, but reject replacement of an
-- established identity. Hard account deletion retains its existing lifecycle
-- through ON DELETE CASCADE; soft deletion preserves the reserved identity.
CREATE FUNCTION calm_guard_claude_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.account_id IS DISTINCT FROM OLD.account_id OR
       (OLD.client_id IS NOT NULL AND NEW.client_id IS DISTINCT FROM OLD.client_id) THEN
        RAISE EXCEPTION 'claude identity replacement forbidden';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER calm_claude_identity_immutable
BEFORE UPDATE ON calm_claude_identities
FOR EACH ROW EXECUTE FUNCTION calm_guard_claude_identity();
