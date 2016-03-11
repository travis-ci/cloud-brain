-- Revert cloudbrain:providers from pg

BEGIN;

DROP TABLE cloudbrain.providers;

COMMIT;
