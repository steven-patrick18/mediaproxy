BEGIN;

-- Pool of signaling (SIP) IPs bound to a SIP-proxy node.
-- Each client gets ONE dedicated signaling IP; carriers whitelist these.
CREATE TABLE signaling_ips (
    id                  BIGSERIAL PRIMARY KEY,
    ip_address          INET   NOT NULL UNIQUE,
    sip_proxy_node_id   BIGINT NOT NULL REFERENCES media_nodes(id) ON DELETE RESTRICT,
    status              TEXT   NOT NULL DEFAULT 'available'
                        CHECK (status IN ('available','assigned','disabled')),
    assigned_client_id  BIGINT REFERENCES clients(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One signaling IP can be assigned to at most one client at a time.
CREATE UNIQUE INDEX uniq_sigip_per_client
    ON signaling_ips(assigned_client_id)
    WHERE assigned_client_id IS NOT NULL;

CREATE INDEX idx_signaling_ips_node ON signaling_ips(sip_proxy_node_id);

-- Each client points back to its assigned signaling IP.
ALTER TABLE clients
    ADD COLUMN signaling_ip_id BIGINT REFERENCES signaling_ips(id) ON DELETE SET NULL;

-- client_ips: dialer source IPs must be globally unique
-- (we resolve client identity by dialer source IP, so duplicates are illegal).
ALTER TABLE client_ips DROP CONSTRAINT IF EXISTS client_ips_client_id_ip_address_key;
ALTER TABLE client_ips ADD CONSTRAINT client_ips_ip_address_key UNIQUE (ip_address);

COMMIT;
