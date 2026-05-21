-- SDP capture: persist the SIP/SDP transport, negotiated media endpoint
-- IP, and (when present) the SDES crypto suite from each call's INVITE.
-- Used by the Privacy Monitor page to surface encryption status and to
-- confirm which carrier media gateway actually carries the audio.
--
-- All three fields are nullable — older calls (and any call where
-- Kamailio doesn't manage to extract SDP for whatever reason) simply
-- show as "unknown" in the UI.

ALTER TABLE active_calls
    ADD COLUMN media_transport   TEXT,
    ADD COLUMN media_endpoint_ip INET,
    ADD COLUMN crypto_suite      TEXT;

ALTER TABLE call_records
    ADD COLUMN media_transport   TEXT,
    ADD COLUMN media_endpoint_ip INET,
    ADD COLUMN crypto_suite      TEXT;

COMMENT ON COLUMN active_calls.media_transport   IS 'SDP m= transport, e.g. RTP/AVP (plain), RTP/SAVP (SDES-SRTP), UDP/TLS/RTP/SAVP (DTLS-SRTP).';
COMMENT ON COLUMN active_calls.media_endpoint_ip IS 'Negotiated SDP c=IN IP4 address — where RTP is actually sent.';
COMMENT ON COLUMN active_calls.crypto_suite      IS 'SDES a=crypto: suite (e.g. AES_CM_128_HMAC_SHA1_80) when present.';
