-- Deploy cloudbrain:providers to pg
-- requires: appschema

BEGIN;

CREATE TABLE cloudbrain.providers (
	id uuid PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	type TEXT NOT NULL,
	config bytea NOT NULL
);

COMMIT;
