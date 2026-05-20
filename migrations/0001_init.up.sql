BEGIN;

CREATE TABLE resellers (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    balance     NUMERIC(14,4) NOT NULL DEFAULT 0,
    settings    JSONB NOT NULL DEFAULT '{}'::jsonb,
    status      TEXT NOT NULL DEFAULT 'active'
                CHECK (status IN ('active','suspended','deleted')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE clients (
    id           BIGSERIAL PRIMARY KEY,
    reseller_id  BIGINT NOT NULL REFERENCES resellers(id) ON DELETE RESTRICT,
    name         TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active','suspended','deleted')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_clients_reseller ON clients(reseller_id);

CREATE TABLE client_ips (
    id          BIGSERIAL PRIMARY KEY,
    client_id   BIGINT NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    ip_address  INET NOT NULL,
    port_range  TEXT,
    status      TEXT NOT NULL DEFAULT 'active'
                CHECK (status IN ('active','disabled')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (client_id, ip_address)
);
CREATE INDEX idx_client_ips_ip ON client_ips(ip_address);

CREATE TABLE admin_users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'admin'
                  CHECK (role IN ('admin','reseller','viewer')),
    mfa_secret    TEXT,
    status        TEXT NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active','suspended')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_log (
    id        BIGSERIAL PRIMARY KEY,
    actor_id  BIGINT REFERENCES admin_users(id) ON DELETE SET NULL,
    action    TEXT NOT NULL,
    target    TEXT,
    before    JSONB,
    after     JSONB,
    ip        INET,
    ts        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_ts    ON audit_log(ts DESC);
CREATE INDEX idx_audit_actor ON audit_log(actor_id);

COMMIT;
