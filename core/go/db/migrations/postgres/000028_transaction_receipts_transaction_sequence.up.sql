BEGIN;

-- Keeps existing UNIQUE ("transaction") index from migration 000015_transaction_receipt_listeners
CREATE INDEX CONCURRENTLY transaction_receipts_transaction_sequence ON transaction_receipts ("transaction", "sequence" DESC);

COMMIT;
