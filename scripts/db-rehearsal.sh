#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -z "${AEGIS_REHEARSAL_DSN_FILE:-}" || ! -f "$AEGIS_REHEARSAL_DSN_FILE" ]]; then
  echo "Set AEGIS_REHEARSAL_DSN_FILE to a mode-0600 DSN for a local aegis_rehearsal_YYYYMMDD_<random hex> schema." >&2
  exit 2
fi
if [[ $# -lt 1 ]]; then
  echo "Usage: scripts/db-rehearsal.sh {restore|status|backup|migrate|verify|samples ...}" >&2
  exit 2
fi
cd "$repo_root/backend"
exec go run ./cmd/db-rehearsal "$@"
