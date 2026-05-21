ALTER TABLE clients
    DROP COLUMN IF EXISTS max_attempts_per_lead,
    DROP COLUMN IF EXISTS rate_limit_window_seconds;
