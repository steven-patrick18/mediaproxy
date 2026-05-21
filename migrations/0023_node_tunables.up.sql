-- Per-node tunables editable from the panel.
--
-- Today these are buried in /etc/node-agent/config.yaml on each box,
-- which means changing them requires SSH + restart. Pushing them into
-- the database + delivering via heartbeat means the operator edits a
-- field in the GUI and the agent picks it up on the next tick.
--
--   kamailio_workers     — count of UDP SIP workers ("children=" in cfg).
--                          NULL means "use the agent default" (16). Bump
--                          to 32-48 for >5k concurrent calls. See TUNING.md.
--   route_cache_seconds  — TTL of the /route cache htable. NULL = agent
--                          default (5s). Set 0 to disable caching for
--                          debugging.
--   route_cache_key_len  — 0 (full DNIS) or N (first N digits). NULL =
--                          agent default (0 / full DNIS).
--
-- All three NULL → agent uses its compiled-in defaults. This keeps
-- existing nodes working unchanged after the migration runs.

ALTER TABLE media_nodes
    ADD COLUMN kamailio_workers     INTEGER,
    ADD COLUMN route_cache_seconds  INTEGER,
    ADD COLUMN route_cache_key_len  INTEGER;

COMMENT ON COLUMN media_nodes.kamailio_workers    IS 'UDP SIP worker count override; NULL = agent default (16).';
COMMENT ON COLUMN media_nodes.route_cache_seconds IS '/route cache TTL in seconds; NULL = agent default (5).';
COMMENT ON COLUMN media_nodes.route_cache_key_len IS 'Leading-digit count of DNIS in cache key; NULL/0 = full DNIS.';
