#!/bin/bash
# This script prepares databases for lightspeed-stack and llama-stack (OGX) with
# postgres_bootstrap.sql.
#
# Note:
# - lightspeed-stack database: Auto-created by container image via POSTGRESQL_DATABASE.
# - llama-stack database: Explicitly created by this script via POSTGRESQL_LLAMA_STACK_DATABASE.
# - POSTGRESQL_ADMIN_PASSWORD is intentionally not set. The postgres superuser has no password
#   by default, which restricts it to local connections only — a deliberate security improvement.
#   Setting POSTGRESQL_ADMIN_PASSWORD would enable remote login for the postgres account.
set -e

cat /var/lib/pgsql/data/userdata/postgresql.conf

echo "Bootstrapping PostgreSQL databases and permissions"

psql \
    -v ON_ERROR_STOP=1 \
    -v postgresql_user="$POSTGRESQL_USER" \
    -v postgresql_lightspeed_stack_database="$POSTGRESQL_DATABASE" \
    -v postgresql_llama_stack_database="$POSTGRESQL_LLAMA_STACK_DATABASE" \
    -f "$POSTGRESQL_BOOTSTRAP_SQL_FILE"
