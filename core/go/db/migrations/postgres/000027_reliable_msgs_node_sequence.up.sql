BEGIN;

-- Supports reliable message drain: WHERE node = ? AND sequence > ? ORDER BY sequence ASC (peer.reliableMessageScan).
-- Leading column "node" still serves filters on node alone (e.g. API queries).
CREATE INDEX reliable_msgs_node_sequence ON reliable_msgs ("node", "sequence");
DROP INDEX reliable_msgs_node;

COMMIT;
