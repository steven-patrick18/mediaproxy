ALTER TABLE active_calls
    DROP COLUMN IF EXISTS avg_jitter_ms,
    DROP COLUMN IF EXISTS avg_packet_loss_pct,
    DROP COLUMN IF EXISTS mos_score;

ALTER TABLE call_records
    DROP COLUMN IF EXISTS avg_jitter_ms,
    DROP COLUMN IF EXISTS avg_packet_loss_pct,
    DROP COLUMN IF EXISTS mos_score;
