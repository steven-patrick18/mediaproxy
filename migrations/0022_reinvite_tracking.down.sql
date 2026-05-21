ALTER TABLE active_calls
    DROP COLUMN IF EXISTS reinvite_count,
    DROP COLUMN IF EXISTS last_reinvite_at,
    DROP COLUMN IF EXISTS last_reinvite_endpoint;

ALTER TABLE call_records
    DROP COLUMN IF EXISTS reinvite_count,
    DROP COLUMN IF EXISTS last_reinvite_at;
