-- Verify cloudbrain:auth_tokens on pg

BEGIN;

SELECT id, description, token_salt, token_hash
FROM cloudbrain.auth_tokens
WHERE false;

ROLLBACK;
