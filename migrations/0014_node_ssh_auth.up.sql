BEGIN;
-- Per-node preferred SSH authentication method, so the Provision modal
-- can default to the right one and the operator doesn't have to toggle
-- every time.
ALTER TABLE media_nodes
    ADD COLUMN ssh_auth_method TEXT NOT NULL DEFAULT 'password'
        CHECK (ssh_auth_method IN ('password','key'));
COMMIT;
