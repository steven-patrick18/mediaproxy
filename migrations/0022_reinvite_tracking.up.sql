-- Mid-call re-INVITE tracking. A re-INVITE on an established dialog is
-- the strongest passively-observable signal of a media path change —
-- a third party joining (conference, eavesdrop bridge), a NAT roam, a
-- carrier forking the leg, or a 200 OK timing on a delayed-offer scenario.
-- We can't tell which from one signal, but the *fact* of a re-INVITE
-- is itself worth surfacing.
--
--   reinvite_count          — number of in-dialog INVITEs observed
--                             AFTER call-start. 0 for normal calls.
--   last_reinvite_at        — timestamp of the most recent re-INVITE.
--                             Used by the Privacy page to surface
--                             "renegotiated N seconds ago" alerts.
--   last_reinvite_endpoint  — media_endpoint_ip from the new SDP, so we
--                             can show the operator "endpoint changed
--                             from X to Y" without a separate audit table.

ALTER TABLE active_calls
    ADD COLUMN reinvite_count         INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN last_reinvite_at       TIMESTAMPTZ,
    ADD COLUMN last_reinvite_endpoint INET;

ALTER TABLE call_records
    ADD COLUMN reinvite_count         INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN last_reinvite_at       TIMESTAMPTZ;

COMMENT ON COLUMN active_calls.reinvite_count         IS 'In-dialog INVITE count observed AFTER initial call-start.';
COMMENT ON COLUMN active_calls.last_reinvite_at       IS 'Timestamp of most recent in-dialog re-INVITE.';
COMMENT ON COLUMN active_calls.last_reinvite_endpoint IS 'media_endpoint_ip from the latest re-INVITE SDP (for change-detection display).';
