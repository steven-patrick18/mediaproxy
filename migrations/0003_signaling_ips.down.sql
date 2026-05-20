BEGIN;
ALTER TABLE clients DROP COLUMN IF EXISTS signaling_ip_id;
DROP TABLE IF EXISTS signaling_ips;
ALTER TABLE client_ips DROP CONSTRAINT IF EXISTS client_ips_ip_address_key;
ALTER TABLE client_ips ADD CONSTRAINT client_ips_client_id_ip_address_key UNIQUE (client_id, ip_address);
COMMIT;
