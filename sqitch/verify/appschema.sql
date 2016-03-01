-- Verify cloudbrain:appschema on pg

BEGIN;

SELECT pg_catalog.has_schema_privilege('cloudbrain', 'usage');d

ROLLBACK;
