BEGIN;

-- A carrier can now be served by multiple media nodes. Routing picks one of
-- the carrier's active nodes per call (priority breaks ties; lower wins).
CREATE TABLE carrier_media_nodes (
    carrier_id   BIGINT      NOT NULL REFERENCES carriers(id)    ON DELETE CASCADE,
    node_id      BIGINT      NOT NULL REFERENCES media_nodes(id) ON DELETE CASCADE,
    priority     INT         NOT NULL DEFAULT 100,
    status       TEXT        NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active','disabled')),
    assigned_by  BIGINT      REFERENCES admin_users(id) ON DELETE SET NULL,
    assigned_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (carrier_id, node_id)
);
CREATE INDEX idx_cmn_node ON carrier_media_nodes(node_id);

-- Migrate any existing single-node assignment into the junction.
INSERT INTO carrier_media_nodes (carrier_id, node_id)
  SELECT id, assigned_node_id
    FROM carriers
   WHERE assigned_node_id IS NOT NULL;

ALTER TABLE carriers DROP COLUMN assigned_node_id;

-- carrier_node_history semantics change: each row now represents one ADD or
-- REMOVE event for a (carrier, node) pair.
--   ADD    => old_node_id IS NULL, new_node_id = the node added
--   REMOVE => old_node_id = the node removed, new_node_id IS NULL
-- The old "reassignment" (old != NULL AND new != NULL) shape is still valid
-- if produced by a future bulk-swap operation. No schema change needed.

COMMIT;
