BEGIN;
ALTER TABLE media_nodes ADD COLUMN firewall_applied_at TIMESTAMPTZ;
COMMIT;
