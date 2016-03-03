-- Deploy cloudbrain:auth_tokens to pg
-- requires: appschema

BEGIN;

CREATE TABLE cloudbrain.auth_tokens (
	id SERIAL PRIMARY KEY,
	description TEXT NOT NULL,
	token_salt bytea NOT NULL,
	token_hash bytea NOT NULL
);

COMMIT;
