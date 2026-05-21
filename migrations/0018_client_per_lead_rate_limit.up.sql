-- Per-client per-lead (DNIS) rate limit. Vicidial-style dialers retry
-- unanswered calls aggressively; without throttling the same destination
-- number gets hit multiple times per minute, which annoys carriers and
-- eventually gets the source media IP blocked as a robocaller.
--
-- max_attempts_per_lead = 0 means "disabled" (default for existing rows so
-- this migration is a no-op for live traffic).
ALTER TABLE clients
    ADD COLUMN max_attempts_per_lead     INT NOT NULL DEFAULT 0,
    ADD COLUMN rate_limit_window_seconds INT NOT NULL DEFAULT 0;

COMMENT ON COLUMN clients.max_attempts_per_lead     IS 'Max times this client may dial the same DNIS per window. 0 = disabled.';
COMMENT ON COLUMN clients.rate_limit_window_seconds IS 'Sliding window in seconds. Ignored when max_attempts_per_lead = 0.';
