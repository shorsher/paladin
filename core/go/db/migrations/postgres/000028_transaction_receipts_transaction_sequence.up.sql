BEGIN;

-- Supports receipt lookups filtered by transaction with ORDER BY sequence (e.g. get-by-id, query API).
-- Keeps existing UNIQUE ("transaction") index for single-row-per-tx enforcement.
CREATE INDEX transaction_receipts_transaction_sequence ON transaction_receipts ("transaction", "sequence" DESC);

COMMIT;
