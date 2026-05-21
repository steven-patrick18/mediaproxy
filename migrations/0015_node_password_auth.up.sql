BEGIN;
-- Tracks the *desired* state of PasswordAuthentication in sshd on this node.
-- The agent reconciles this on apply: edits /etc/ssh/sshd_config, validates
-- with `sshd -t`, then `systemctl reload ssh`.
ALTER TABLE media_nodes ADD COLUMN password_auth_enabled BOOLEAN NOT NULL DEFAULT false;
COMMIT;
