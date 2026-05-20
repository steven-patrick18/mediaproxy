BEGIN;
DROP TABLE IF EXISTS active_calls;
DROP TABLE IF EXISTS call_records;
ALTER TABLE carriers  DROP COLUMN IF EXISTS notes;
ALTER TABLE clients   DROP COLUMN IF EXISTS notes;
ALTER TABLE resellers DROP COLUMN IF EXISTS notes;
ALTER TABLE admin_users DROP CONSTRAINT IF EXISTS admin_users_role_check;
ALTER TABLE admin_users ADD CONSTRAINT admin_users_role_check
    CHECK (role IN ('admin','reseller','viewer'));
ALTER TABLE node_ips DROP COLUMN IF EXISTS auto_discovered;
COMMIT;
