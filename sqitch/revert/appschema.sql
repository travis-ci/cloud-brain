-- Revert cloudbrain:appschema from pg

BEGIN;

DROP SCHEMA cloudbrain;

COMMIT;
