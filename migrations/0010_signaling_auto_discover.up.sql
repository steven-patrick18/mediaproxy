BEGIN;
ALTER TABLE signaling_ips
    ADD COLUMN auto_discovered BOOLEAN NOT NULL DEFAULT false;
COMMIT;
