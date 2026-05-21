ALTER TABLE media_nodes ADD COLUMN IF NOT EXISTS password_auth_enabled BOOLEAN NOT NULL DEFAULT false;
