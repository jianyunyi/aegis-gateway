#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE="$ROOT_DIR/.env"

if [ -e "$ENV_FILE" ]; then
  echo "Refusing to overwrite existing $ENV_FILE" >&2
  exit 1
fi
if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl is required to generate random secrets" >&2
  exit 1
fi

mysql_root_password=$(openssl rand -hex 24)
mysql_password=$(openssl rand -hex 24)
redis_password=$(openssl rand -hex 24)
jwt_secret=$(openssl rand -hex 32)
admin_password=$(openssl rand -hex 16)

umask 077
cat > "$ENV_FILE" <<EOF
MYSQL_USER=aegis
MYSQL_ROOT_PASSWORD=$mysql_root_password
MYSQL_PASSWORD=$mysql_password
REDIS_PASSWORD=$redis_password
JWT_SECRET=$jwt_secret
SEED_DATA=true
BOOTSTRAP_ADMIN_USERNAME=admin
BOOTSTRAP_ADMIN_PASSWORD=$admin_password
PROMPT_CAPTURE_ENABLED=false
PROMPT_RETENTION_DAYS=30
MAX_REQUEST_BODY_BYTES=2097152
CORS_ALLOWED_ORIGINS=
EOF
chmod 600 "$ENV_FILE"

printf 'Generated local Compose secrets in %s\n' "$ENV_FILE"
printf 'Initial admin username: admin\nInitial admin password: %s\n' "$admin_password"
printf 'Save the password now; it will not be printed again. Do not use this local .env in production.\n'
