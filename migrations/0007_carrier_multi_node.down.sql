BEGIN;
ALTER TABLE carriers ADD COLUMN assigned_node_id BIGINT REFERENCES media_nodes(id) ON DELETE SET NULL;
UPDATE carriers c
   SET assigned_node_id = (
     SELECT node_id FROM carrier_media_nodes
       WHERE carrier_id = c.id AND status = 'active'
       ORDER BY priority, node_id LIMIT 1
   );
DROP TABLE carrier_media_nodes;
COMMIT;
