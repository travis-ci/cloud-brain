-- Deploy cloudbrain:providers to pg
-- requires: appschema

BEGIN;

CREATE TABLE cloudbrain.providers (
	id uuid PRIMARY KEY,
	type TEXT NOT NULL,
	config bytea NOT NULL
);

COMMIT;
