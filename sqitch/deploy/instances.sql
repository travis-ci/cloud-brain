-- Deploy cloudbrain:instances to pg
-- requires: appschema

BEGIN;

CREATE TABLE cloudbrain.instances (
	id          uuid  PRIMARY KEY,
	provider    TEXT  NOT NULL,
	provider_id TEXT,
	image       TEXT  NOT NULL,
	state       TEXT  NOT NULL,
	ip_address  TEXT,
	ssh_key     TEXT
);

COMMIT;
