-- The Settings -> Base-app SSH and Nodes -> SSH password toggle feature is
-- being removed. SSH password authentication is now controlled directly on
-- each host via /etc/ssh/sshd_config (panel no longer manages it).

ALTER TABLE media_nodes DROP COLUMN IF EXISTS password_auth_enabled;
