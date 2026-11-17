#!/usr/bin/env bash
# Local synthetic checks only; opt-in database integration is a separate step.
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "$0")/../.." && pwd)
unset CALM_IDENTITY_TEST_DSN
cd "$ROOT/backend"
go test -mod=readonly -tags unit ./internal/service ./internal/repository ./migrations \
  -run 'Test(Calm|Identity|Fingerprint|DefaultFingerprint|Build.*|GatewayService.*|ForwardCountTokens.*|NormalizeClaudeOAuth.*|ApplyClaude.*)' \
  -count=1 -timeout=180s
go test -mod=readonly -tags unit ./internal/handler/admin ./internal/server/routes \
  -run '^TestCalm' -count=1 -timeout=180s
go test -mod=readonly -race -tags unit ./internal/service ./internal/repository \
  -run '^TestCalm' -count=1 -timeout=180s
