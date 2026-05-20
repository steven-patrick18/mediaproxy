BEGIN;

-- Media nodes (and SIP proxies — same table, different role)
CREATE TABLE media_nodes (
    id                  BIGSERIAL PRIMARY KEY,
    name                TEXT NOT NULL UNIQUE,
    role                TEXT NOT NULL DEFAULT 'media'
                        CHECK (role IN ('media','sip_proxy')),
    host_ip             INET NOT NULL,
    mgmt_ip             INET,
    region              TEXT,
    cpu_cores           INT,
    ram_gb              INT,
    nic_gbps            INT,
    max_calls           INT NOT NULL DEFAULT 0,
    soft_warn           INT,
    hard_limit          INT,
    transcoding_enabled BOOLEAN NOT NULL DEFAULT false,
    status              TEXT NOT NULL DEFAULT 'offline'
                        CHECK (status IN ('online','offline','draining')),
    agent_token         TEXT NOT NULL UNIQUE,
    rtpengine_version   TEXT,
    last_seen_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_media_nodes_role   ON media_nodes(role);
CREATE INDEX idx_media_nodes_status ON media_nodes(status);

-- Leased IPs bound to a node
CREATE TABLE node_ips (
    id                BIGSERIAL PRIMARY KEY,
    node_id           BIGINT NOT NULL REFERENCES media_nodes(id) ON DELETE CASCADE,
    ip_address        INET NOT NULL UNIQUE,
    status            TEXT NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active','disabled','flagged','reserve')),
    purchased_from    TEXT,
    lease_block       TEXT,
    lease_expires     DATE,
    monthly_cost      NUMERIC(10,4),
    rdns              TEXT,
    reputation_score  INT,
    last_health_check TIMESTAMPTZ,
    current_calls     INT NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_node_ips_node   ON node_ips(node_id);
CREATE INDEX idx_node_ips_status ON node_ips(status);

-- IP groups (named subsets of node_ips assigned to a client+carrier)
CREATE TABLE ip_groups (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    status      TEXT NOT NULL DEFAULT 'active'
                CHECK (status IN ('active','paused','ended')),
    notes       TEXT,
    created_by  BIGINT REFERENCES admin_users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ip_group_members (
    id        BIGSERIAL PRIMARY KEY,
    group_id  BIGINT NOT NULL REFERENCES ip_groups(id) ON DELETE CASCADE,
    ip_id     BIGINT NOT NULL REFERENCES node_ips(id)  ON DELETE CASCADE,
    active    BOOLEAN NOT NULL DEFAULT true
);
-- one IP can be in only ONE active group at a time (ARCHITECTURE §4 rule)
CREATE UNIQUE INDEX uq_ip_group_active ON ip_group_members(ip_id) WHERE active = true;
CREATE INDEX idx_ip_group_members_group ON ip_group_members(group_id);

-- Carriers (upstream termination)
CREATE TABLE carriers (
    id                BIGSERIAL PRIMARY KEY,
    name              TEXT NOT NULL UNIQUE,
    host              TEXT NOT NULL,
    port              INT  NOT NULL DEFAULT 5060,
    transport         TEXT NOT NULL DEFAULT 'udp'
                      CHECK (transport IN ('udp','tcp','tls')),
    assigned_node_id  BIGINT REFERENCES media_nodes(id) ON DELETE SET NULL,
    codec_pref        TEXT,
    status            TEXT NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active','paused','disabled')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_carriers_node ON carriers(assigned_node_id);

-- History of carrier-to-node reassignments (ARCHITECTURE §4 rule)
CREATE TABLE carrier_node_history (
    id                      BIGSERIAL PRIMARY KEY,
    carrier_id              BIGINT NOT NULL REFERENCES carriers(id) ON DELETE CASCADE,
    old_node_id             BIGINT REFERENCES media_nodes(id) ON DELETE SET NULL,
    new_node_id             BIGINT REFERENCES media_nodes(id) ON DELETE SET NULL,
    changed_by              BIGINT REFERENCES admin_users(id) ON DELETE SET NULL,
    changed_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    reason                  TEXT,
    active_calls_at_switch  INT
);
CREATE INDEX idx_cnh_carrier ON carrier_node_history(carrier_id, changed_at DESC);

-- Routes: client+destination -> carrier
CREATE TABLE routes (
    id            BIGSERIAL PRIMARY KEY,
    client_id     BIGINT NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    match_prefix  TEXT,
    carrier_id    BIGINT NOT NULL REFERENCES carriers(id) ON DELETE RESTRICT,
    priority      INT NOT NULL DEFAULT 100,
    status        TEXT NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active','disabled'))
);
CREATE INDEX idx_routes_client ON routes(client_id, priority);

-- Active client/carrier -> ip_group assignments
CREATE TABLE assignments (
    id                BIGSERIAL PRIMARY KEY,
    group_id          BIGINT NOT NULL REFERENCES ip_groups(id) ON DELETE RESTRICT,
    client_id         BIGINT NOT NULL REFERENCES clients(id)  ON DELETE CASCADE,
    carrier_id        BIGINT NOT NULL REFERENCES carriers(id) ON DELETE RESTRICT,
    rotation_strategy TEXT NOT NULL DEFAULT 'round_robin'
                      CHECK (rotation_strategy IN ('round_robin','random','sticky','least_used','health_weighted')),
    status            TEXT NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active','paused','ended')),
    assigned_by       BIGINT REFERENCES admin_users(id) ON DELETE SET NULL,
    assigned_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- one active assignment per (client, carrier)
CREATE UNIQUE INDEX uq_assignment_active
    ON assignments(client_id, carrier_id) WHERE status = 'active';
CREATE INDEX idx_assignments_group ON assignments(group_id);

COMMIT;
