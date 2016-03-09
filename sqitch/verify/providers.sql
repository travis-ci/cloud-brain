-- Verify cloudbrain:providers on pg

BEGIN;

SELECT id, type, config
FROM cloudbrain.providers
WHERE false;

ROLLBACK;
