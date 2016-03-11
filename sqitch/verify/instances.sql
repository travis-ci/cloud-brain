-- Verify cloudbrain:instances on pg

BEGIN;

SELECT id, provider_name, image, state, ip_address, ssh_key
FROM cloudbrain.instances
WHERE false;

ROLLBACK;
