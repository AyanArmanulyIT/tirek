-- Dev-only role provisioning (docker-compose local environment).
-- Production roles are created by infrastructure (RDS + Secrets Manager);
-- this script only exists so a fresh `docker compose up` has the low-privilege
-- application role available before migrations grant it privileges.

CREATE ROLE tirek_app LOGIN PASSWORD 'tirek_app';
CREATE ROLE tirek_migrator LOGIN BYPASSRLS PASSWORD 'tirek_migrator';