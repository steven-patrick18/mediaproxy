-- Phase 3 of route-quality monitoring: per-call RTP quality metrics
-- sampled from RTPEngine's NG control socket.
--
--   avg_jitter_ms        — latest reported inter-arrival jitter (ms).
--                          RTPEngine reports per-stream jitter; we average
--                          across the call's media streams. <30ms = good,
--                          >50ms = audibly bad.
--   avg_packet_loss_pct  — latest reported loss percentage. <1% = good,
--                          >2% = audibly bad on G.711, much sooner on G.729.
--   mos_score            — derived MOS estimate using a simplified E-model
--                          off the jitter + loss numbers. 4.0+ = good,
--                          3.5–4.0 = OK, <3.5 = poor.
--
-- All three are nullable — calls older than Phase 3 (and any call where
-- the agent couldn't reach RTPEngine) just show as "—".

ALTER TABLE active_calls
    ADD COLUMN avg_jitter_ms        FLOAT,
    ADD COLUMN avg_packet_loss_pct  FLOAT,
    ADD COLUMN mos_score            FLOAT;

ALTER TABLE call_records
    ADD COLUMN avg_jitter_ms        FLOAT,
    ADD COLUMN avg_packet_loss_pct  FLOAT,
    ADD COLUMN mos_score            FLOAT;

COMMENT ON COLUMN active_calls.avg_jitter_ms       IS 'Inter-arrival jitter in ms, averaged across media streams.';
COMMENT ON COLUMN active_calls.avg_packet_loss_pct IS 'Packet loss percentage as reported by RTPEngine.';
COMMENT ON COLUMN active_calls.mos_score           IS 'Derived MOS estimate (1.0–4.5) from a simplified E-model on jitter+loss.';
