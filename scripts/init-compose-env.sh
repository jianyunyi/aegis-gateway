#!/bin/sh
set -eu
ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE="$ROOT_DIR/.env"
SECRETS_DIR="$ROOT_DIR/.secrets"
if [ -e "$ENV_FILE" ] || [ -e "$SECRETS_DIR" ]; then
  echo "Refusing to overwrite existing $ENV_FILE or $SECRETS_DIR" >&2
  exit 1
fi
if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl is required to generate random secrets" >&2
  exit 1
fi

umask 077
secrets_tmp=$(mktemp -d "$ROOT_DIR/.secrets.tmp.XXXXXX")
env_tmp="$ROOT_DIR/.env.tmp.$$"
cleanup() {
  rm -rf "$secrets_tmp"
  rm -f "$env_tmp"
}
trap cleanup EXIT HUP INT TERM

mysql_root_password=$(openssl rand -hex 24)
mysql_password=$(openssl rand -hex 24)
redis_password=$(openssl rand -hex 24)
jwt_secret=$(openssl rand -hex 32)
admin_password=$(openssl rand -hex 16)

printf '%s\n' "$mysql_root_password" > "$secrets_tmp/mysql_root_password"
printf '%s\n' "$mysql_password" > "$secrets_tmp/mysql_password"
printf '%s\n' "$redis_password" > "$secrets_tmp/redis_password"
printf '%s\n' "$jwt_secret" > "$secrets_tmp/jwt_secret"
printf '%s\n' "$admin_password" > "$secrets_tmp/bootstrap_admin_password"
printf 'aegis:%s@tcp(mysql:3306)/aegis?charset=utf8mb4&parseTime=True&loc=Local\n' "$mysql_password" > "$secrets_tmp/mysql_dsn"
printf 'aegis:%s@tcp(127.0.0.1:3303)/aegis?charset=utf8mb4&parseTime=True&loc=Local\n' "$mysql_password" > "$secrets_tmp/mysql_dsn_host"
printf 'requirepass %s\n' "$redis_password" > "$secrets_tmp/redis.conf"
chmod 600 "$secrets_tmp"/*
# The official Redis image drops privileges before reading its config file.
# Keep the file world-readable inside its mounted container, while its parent
# directory remains mode 0700 on the host and only the Redis service mounts it.
chmod 644 "$secrets_tmp/redis.conf"
chmod 700 "$secrets_tmp"

cat > "$env_tmp" <<EOF
MYSQL_USER=aegis
SECRETS_DIR=../.secrets
APP_UID=$(id -u)
APP_GID=$(id -g)
SEED_DATA=true
BOOTSTRAP_ADMIN_USERNAME=admin
PROMPT_CAPTURE_ENABLED=false
PROMPT_RETENTION_DAYS=30
MAX_REQUEST_BODY_BYTES=2097152
CORS_ALLOWED_ORIGINS=
EOF
chmod 600 "$env_tmp"
mv "$secrets_tmp" "$SECRETS_DIR"
mv "$env_tmp" "$ENV_FILE"
trap - EXIT HUP INT TERM
printf 'Generated local Compose secret files in %s (mode 0600)\n' "$SECRETS_DIR"
printf 'Initial admin username: admin\nInitial admin password: %s\n' "$admin_password"
printf 'Save the password now; it is also stored locally in the protected bootstrap secret file. Do not use these local credentials in production.\n'
