ALTER TABLE media_nodes
    DROP COLUMN IF EXISTS kamailio_workers,
    DROP COLUMN IF EXISTS route_cache_seconds,
    DROP COLUMN IF EXISTS route_cache_key_len;
