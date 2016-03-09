-- Verify cloudbrain:instances on pg

BEGIN;

SELECT id, provider_type, provider_id, image, state, ip_address, ssh_key
FROM cloudbrain.instances
WHERE false;

ROLLBACK;
