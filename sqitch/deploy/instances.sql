-- Deploy cloudbrain:instances to pg
-- requires: appschema providers

BEGIN;

CREATE TABLE cloudbrain.instances (
	id            uuid  PRIMARY KEY,
	provider_name TEXT  NOT NULL REFERENCES cloudbrain.providers(name) ON UPDATE CASCADE,
	image         TEXT  NOT NULL,
	state         TEXT  NOT NULL,
	ip_address    TEXT,
	ssh_key       TEXT
);

COMMIT;
