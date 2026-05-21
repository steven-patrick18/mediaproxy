ALTER TABLE active_calls
    DROP COLUMN IF EXISTS pdd_ms,
    DROP COLUMN IF EXISTS codecs_offered;

ALTER TABLE call_records
    DROP COLUMN IF EXISTS pdd_ms,
    DROP COLUMN IF EXISTS codecs_offered;
