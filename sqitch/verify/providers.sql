-- Verify cloudbrain:providers on pg

BEGIN;

SELECT id, type, name, config
FROM cloudbrain.providers
WHERE false;

ROLLBACK;
