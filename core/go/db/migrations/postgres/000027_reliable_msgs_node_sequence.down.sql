BEGIN;

CREATE INDEX CONCURRENTLY reliable_msgs_node ON reliable_msgs ("node");
DROP INDEX reliable_msgs_node_sequence;

COMMIT;
