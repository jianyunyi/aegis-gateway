#!/usr/bin/env bash
set -euo pipefail

mode="${1:-}"
if [[ "$mode" != "outside" && "$mode" != "inside" ]]; then
  echo "Usage: PUBLIC_API_URL=https://api.example.com ADMIN_URL=https://aegis-admin.<tailnet>.ts.net $0 {outside|inside}" >&2
  exit 2
fi
: "${PUBLIC_API_URL:?set the public HTTPS API origin, e.g. https://api.example.com}"
: "${ADMIN_URL:?set the Tailscale Serve admin URL, e.g. https://aegis-admin.example.ts.net}"

http_code() {
  local url="$1"
  local code
  if code="$(curl --noproxy '*' --connect-timeout 5 --max-time 10 --silent --show-error --output /dev/null --write-out '%{http_code}' "$url" 2>/dev/null)"; then
    printf '%s' "$code"
  else
    printf '000'
  fi
}

api_code="$(http_code "${PUBLIC_API_URL%/}/v1/models")"
if [[ "$api_code" != "401" && "$api_code" != "403" ]]; then
  echo "FAIL: public API /v1/models returned HTTP $api_code; expected reachable API auth response 401/403." >&2
  exit 1
fi
login_code="$(curl --noproxy '*' --connect-timeout 5 --max-time 10 --silent --show-error --output /dev/null --write-out '%{http_code}' \
  --request POST --header 'Content-Type: application/json' --data '{}' \
  "${PUBLIC_API_URL%/}/api/v1/admin/auth/login" 2>/dev/null || printf '000')"
public_ui_code="$(http_code "${PUBLIC_API_URL%/}/login")"

if [[ "$login_code" != "404" ]]; then
  echo "FAIL: public admin login path returned HTTP $login_code; expected 404 from the API-only public proxy." >&2
  exit 1
fi
if [[ "$public_ui_code" != "404" ]]; then
  echo "FAIL: public /login returned HTTP $public_ui_code; expected 404 from the API-only public proxy." >&2
  exit 1
fi

admin_code="$(http_code "${ADMIN_URL%/}/login")"
if [[ "$mode" == "outside" ]]; then
  if [[ "$admin_code" != "000" && "$admin_code" != "403" && "$admin_code" != "404" ]]; then
    echo "FAIL: admin login page returned HTTP $admin_code outside the VPN; expected network denial or 403/404." >&2
    exit 1
  fi
  echo "PASS: public API is reachable; public admin login and /login are 404; admin UI is denied outside the VPN (HTTP $admin_code)."
  exit 0
fi

if [[ "$admin_code" != "200" ]]; then
  echo "FAIL: admin login page returned HTTP $admin_code from inside the VPN; expected 200." >&2
  exit 1
fi
echo "PASS: admin login page is reachable through VPN HTTPS; public admin login and /login remain 404. Now sign in interactively and confirm the dashboard loads."
