-- Verify cloudbrain:instances on pg

BEGIN;

SELECT id, provider_name, image, state, ip_address, ssh_key, upstream_id
FROM cloudbrain.instances
WHERE false;

ROLLBACK;
