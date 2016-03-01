-- Revert cloudbrain:instances from pg

BEGIN;

DROP TABLE cloudbrain.instances;

COMMIT;
