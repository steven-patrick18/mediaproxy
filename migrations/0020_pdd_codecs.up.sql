-- Phase 2 of route-quality monitoring.
--   pdd_ms        : post-dial delay in milliseconds. Time from when
--                   Kamailio relayed the INVITE outbound to when the
--                   carrier's first 18x reply (180 Ringing or 183
--                   Session Progress) arrived. Tier-1 routes land in
--                   1000-3000 ms; grey routes commonly 5000-15000 ms.
--   codecs_offered: comma-separated list of codecs from the offerer's
--                   SDP (m= payload types + a=rtpmap: names). Used to
--                   spot "codec lock" patterns (e.g. carrier only ever
--                   accepts G.729, suggesting bandwidth-constrained
--                   backhaul).

ALTER TABLE active_calls
    ADD COLUMN pdd_ms          INTEGER,
    ADD COLUMN codecs_offered  TEXT;

ALTER TABLE call_records
    ADD COLUMN pdd_ms          INTEGER,
    ADD COLUMN codecs_offered  TEXT;

COMMENT ON COLUMN active_calls.pdd_ms         IS 'Post-dial delay in ms (INVITE -> first 18x). NULL if 18x never arrived.';
COMMENT ON COLUMN active_calls.codecs_offered IS 'Comma-separated codec list from SDP (e.g. PCMU/8000,PCMA/8000,G729/8000).';
