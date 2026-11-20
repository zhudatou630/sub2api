# v0.2.24 integration review

## Status and source

Merged into `calm` and tagged `calm-v0.2.24-identity.1`. The tag identifies source only: it publishes no image, and image publication and deployment remain separate operations.

- Fork starting point: `f52f08b22` (`calm`, checkpoint `calm-v0.2.22-identity.1` plus documentation).
- Reviewed upstream tag: `v0.2.24`, peeled commit `5c2982a81b46e2142d0143d2f83c1ce9c4da1d24` (142 commits after `v0.2.22`). The later upstream `main` commit only syncs `VERSION`.
- Integration branch: `update/v0.2.24`; upstream merged with `--no-ff`, then `calm` fast-forwarded. No textual conflicts; no fork source change was needed.
- The review itself performed no push, tag, CI publication or deployment. Merging, tagging and pushing were done afterwards as separate authorized steps; CI remains disabled at repository level.

## Semantic review

Clean auto-merge was not treated as compatibility. Upstream changes were checked against the fork's identity and maintenance boundaries:

1. **Identity consumers.** Auto-merged files that also carry fork changes (`account_test_service.go`, `account_usage_service.go`, `gateway_service.go`, `gateway_claude_oauth_body.go`, `gateway_forward_as_*.go`, `upstream_models.go`) only gain temperature gating for models that reject it, quota-window statistics, usage/cache-token accounting, 5m cache TTL for Claude Code compaction requests, and a models.dev host match. None touches ClientID storage, header bundles, metadata rewriting or session derivation.
2. **Request parsing.** `metadata.user_id` is now read in a single root scan with the same first-resolvable-key semantics as before. A new route middleware rejects ambiguous duplicate fields; `metadata` is deliberately excluded, so identity rewriting is unaffected.
3. **Wire/constructors.** The only constructor change adds a quota repository to `NewAdminService`; identity injection is unchanged.
4. **Default bundle.** No change under `internal/pkg/claude` except a new `RejectsTemperature` helper; `DefaultHeaders()` and the CLI version source are untouched.
5. **OAuth refresh.** New conditional-persistence logic applies only to OpenAI OAuth accounts.
6. **New outbound paths.** No new Anthropic OAuth/setup-token request path was introduced. The new Exa web-search client and OpenAI paths are outside the Claude identity scope.
7. **Dangerous maintenance.** No upstream change to update/rollback/restart/restore/cleanup routes, handlers or frontend controls; the fork's `403 DANGEROUS_MAINTENANCE_DISABLED` boundaries remain and their tests pass.
8. **Migrations.** One new file, `252_channel_pricing_ultrafast_multiplier.sql`: nullable column plus a `CHECK` constraint on `channel_model_pricing` (validation scans that small table under a short exclusive lock). No existing SQL changed; `900001_calm_claude_identity.sql` is byte-identical.

## Verification environment

- Go `1.27.4` (exact `go.mod` version), `-mod=readonly`; `gcc` for race tests.
- Frontend: Node `24.22.3` and pnpm `9.15.9`, matching the Dockerfile/CI major versions, kept in a private toolchain directory; frozen lockfile, lifecycle scripts disabled, no lockfile change.
- Backend/frontend tests and database work ran in a separate network namespace with only namespace-local loopback and proxy variables unset. No host service, production database/Redis or real model was used.
- Database: disposable PostgreSQL `17.12` from checksum-verified Debian packages, unpacked (not installed), run as an unprivileged user inside the namespace and discarded afterwards.

## Results

| Check | Result |
|---|---|
| `bash scripts/calm/check.sh` | PASS |
| `go test -p 3 -tags unit ./...` (network-isolated) | PASS: 61 test-bearing packages, 50 without tests; no hosts fixture needed this time |
| Frontend full Vitest | PASS: 389 files, 3192 tests |
| Frontend `pnpm run build` (locale completeness, `vue-tsc -b`, Vite) | PASS; existing large-chunk/deprecation warnings |
| Backend `-tags embed` build, explicit version `0.2.24-calm.1-dev` | PASS; binary not run or deployed |
| Embedded web tests (`-tags 'unit embed'`) | PASS |
| Real PostgreSQL identity contract, plain and `-race` | PASS |
| Fresh database: two concurrent `ApplyMigrations` + repeat | PASS: 306 filenames, checksums match |
| Upgrade from the `calm` 305-file baseline with synthetic identities | PASS: only `252` applied; established, reserved-NULL and soft-deleted identities unchanged; immutability trigger still rejects replacement |

The database rehearsal used a temporary, uncommitted test harness.

## Limits

Not validated: production topology, data volume or lock contention, PostgreSQL 18 (production major version), Docker image build, live provider behavior, container-based integration tests, browser E2E and rollback rehearsal. `backend/cmd/server/VERSION` in the upstream tag is `0.2.23`; builds must pass an explicit fork version. Deployment, database maintenance and restart require separate authorization.
