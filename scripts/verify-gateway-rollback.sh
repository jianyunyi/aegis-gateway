#!/usr/bin/env bash
set -euo pipefail

: "${PREVIOUS_GATEWAY_IMAGE:?set PREVIOUS_GATEWAY_IMAGE to the previously deployed local image:tag or digest}"
: "${AEGIS_REHEARSAL_DSN_FILE:?set AEGIS_REHEARSAL_DSN_FILE to a mode-0600 local rehearsal DSN}"
: "${REHEARSAL_REDIS_PASSWORD_FILE:?set REHEARSAL_REDIS_PASSWORD_FILE to a mode-0600 rehearsal Redis password file}"
: "${REHEARSAL_JWT_SECRET_FILE:?set REHEARSAL_JWT_SECRET_FILE to a mode-0600 rehearsal JWT secret file}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ "$(id -u)" == "0" ]]; then
  echo "Run this rehearsal as an unprivileged local user, not root." >&2
  exit 2
fi
rollback_addr="${AEGIS_ROLLBACK_HTTP_ADDR:-127.0.0.1:18080}"
if [[ ! "$rollback_addr" =~ ^127\.0\.0\.1:[1-9][0-9]{3,4}$ ]]; then
  echo "Refusing non-loopback or privileged HTTP address: $rollback_addr" >&2
  exit 2
fi
for secret_file in "$AEGIS_REHEARSAL_DSN_FILE" "$REHEARSAL_REDIS_PASSWORD_FILE" "$REHEARSAL_JWT_SECRET_FILE"; do
  [[ -f "$secret_file" ]] || { echo "Missing rehearsal secret file: $secret_file" >&2; exit 2; }
  mode="$(stat -c '%a' "$secret_file")"
  [[ "$mode" == "600" ]] || { echo "Rehearsal secret must have mode 0600: $secret_file" >&2; exit 2; }
done

# The database tool rejects remote hosts and all schema names outside aegis_rehearsal*.
AEGIS_REHEARSAL_DSN_FILE="$AEGIS_REHEARSAL_DSN_FILE" \
  "$repo_root/scripts/db-rehearsal.sh" status >/dev/null
command -v docker >/dev/null || { echo "docker is required for the rollback smoke test" >&2; exit 2; }
docker image inspect "$PREVIOUS_GATEWAY_IMAGE" >/dev/null || {
  echo "Previous gateway image is not present locally; refusing to pull or access a registry." >&2
  exit 2
}

secret_dir="$(mktemp -d)"
chmod 700 "$secret_dir"
cp "$AEGIS_REHEARSAL_DSN_FILE" "$secret_dir/mysql_dsn"
cp "$REHEARSAL_REDIS_PASSWORD_FILE" "$secret_dir/redis_password"
cp "$REHEARSAL_JWT_SECRET_FILE" "$secret_dir/jwt_secret"
chmod 600 "$secret_dir"/*
container_name="aegis-rollback-rehearsal-$$"
cleanup() {
  docker stop "$container_name" >/dev/null 2>&1 || true
  rm -rf "$secret_dir"
}
trap cleanup EXIT

redis_addr="${REHEARSAL_REDIS_ADDR:-127.0.0.1:6380}"
docker run --detach --rm \
  --pull never \
  --name "$container_name" \
  --network host \
  --user "$(id -u):$(id -g)" \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --mount "type=bind,src=$secret_dir,dst=/tmp/aegis-rehearsal-secrets,readonly" \
  --env APP_ENV=prod \
  --env HTTP_ADDR="$rollback_addr" \
  --env MYSQL_DSN_FILE=/tmp/aegis-rehearsal-secrets/mysql_dsn \
  --env REDIS_ADDR="$redis_addr" \
  --env REDIS_PASSWORD_FILE=/tmp/aegis-rehearsal-secrets/redis_password \
  --env JWT_SECRET_FILE=/tmp/aegis-rehearsal-secrets/jwt_secret \
  --env AUTO_MIGRATE=false \
  --env RATE_LIMIT_FAIL_OPEN=false \
  --env PROMPT_CAPTURE_ENABLED=false \
  --env SEED_DATA=false \
  "$PREVIOUS_GATEWAY_IMAGE" >/dev/null

base_url="http://$rollback_addr"
ready=false
for _ in $(seq 1 30); do
  if curl --fail --silent --max-time 2 "$base_url/readyz" >/dev/null; then
    ready=true
    break
  fi
  sleep 1
done
if [[ "$ready" != true ]]; then
  docker logs "$container_name" >&2 || true
  echo "Previous gateway did not become ready against the isolated migrated database." >&2
  exit 1
fi
api_code="$(curl --noproxy '*' --max-time 5 --silent --output /dev/null --write-out '%{http_code}' "$base_url/v1/models" || true)"
if [[ "$api_code" != "401" && "$api_code" != "403" ]]; then
  echo "Previous gateway /v1/models returned HTTP $api_code; expected an unauthenticated API rejection (401/403)." >&2
  exit 1
fi
echo "PASS: previous gateway image started on loopback against the local isolated rehearsal database and Redis; /readyz passed and /v1/models returned $api_code. No image was pulled and no production endpoint was contacted."
