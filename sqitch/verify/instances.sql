-- Verify cloudbrain:instances on pg

BEGIN;

SELECT id, provider, provider_id, image, state, ip_address
FROM cloudbrain.instances
WHERE false;

ROLLBACK;
