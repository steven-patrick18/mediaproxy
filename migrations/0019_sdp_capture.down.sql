ALTER TABLE active_calls
    DROP COLUMN IF EXISTS media_transport,
    DROP COLUMN IF EXISTS media_endpoint_ip,
    DROP COLUMN IF EXISTS crypto_suite;

ALTER TABLE call_records
    DROP COLUMN IF EXISTS media_transport,
    DROP COLUMN IF EXISTS media_endpoint_ip,
    DROP COLUMN IF EXISTS crypto_suite;
