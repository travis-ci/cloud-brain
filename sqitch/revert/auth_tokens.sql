-- Revert cloudbrain:auth_tokens from pg

BEGIN;

DROP TABLE cloudbrain.auth_tokens;

COMMIT;
