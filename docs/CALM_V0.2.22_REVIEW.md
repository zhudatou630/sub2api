# v0.2.22 integration candidate review

## Status and source

This is an unpublished source integration candidate, not a release or deployment approval.

- Fork starting point: `6f1edbf8c338338dd036b85161b57ab6eb0ccc24` (`calm`).
- Reviewed upstream tag: `v0.2.22`.
- Upstream commit (peeled tag): `b0fa9f3e6eeab1968a53e1289b69daa4bb40ca2b`.
- Integration branch: `update/v0.2.22`; merge prepared with `--no-ff --no-commit`.
- The compact fork README and deletion of translated promotional READMEs are retained. Newly introduced promotional artwork is not restored.
- No commit, push, release tag, CI publication or deployment was performed as part of this review.

## Changes beyond upstream

### Claude hardening

The existing PostgreSQL identity store and immutable identity migration remain authoritative. The review additionally fixes:

1. Legacy account identity fields and missing `account_uuid` cannot cause downstream device/account identity to supersede the persisted winner.
2. Anthropic OAuth/setup-token metadata passthrough cannot bypass identity rewriting. The existing session derivation formula is unchanged, but raw downstream sessions are no longer preserved by this option for these account types.
3. Native Claude/count_tokens requests receive the full server-owned default header bundle, including retry/timeout/application declarations. OAuth count_tokens mimicry skips downstream headers like messages does.
4. Non-Claude-Code count_tokens mimicry synthesizes missing metadata from the persisted identity and keeps generation-only fields out of the counting request.
5. Administrative Claude connection/model probes now require the same identity service. Reserved NULL, invalid identity, missing service and storage errors stop before upstream access.
6. Token-rotation retries retain the identity policy and stop if identity becomes unavailable between attempts. Affected dependency error strings are not emitted in diagnostic logs.

API-key/non-Claude behavior is retained; test fixtures verify those boundaries. No credential, egress, scheduling or TLS algorithm changes were introduced by the fork-specific fixes. Upstream v0.2.22 changes in those areas remain part of the upstream integration.

### Dangerous Web/API maintenance

Backend handlers/routes reject the following with `403` and reason `DANGEROUS_MAINTENANCE_DISABLED`, without an administrator opt-in:

- Application update, rollback selection/execution and running-service restart.
- Whole-database restoration from backup.
- Bulk audit-log clearing, creation of usage-cleanup tasks and manual system-log cleanup.

The version badge is display-only. Version/check-update queries return local metadata without consulting GitHub or updater caches. Frontend controls are removed/disabled, and negative tests cover backend rejection independently of the UI. The three log/usage clearing handlers reject directly as well as at their routes, preventing accidental rebinding of the original handler from restoring the operation.

Ops retention `0` skips cleanup rather than truncating tables, including old persisted zero settings. Positive-period expiry remains. **This can retain more data and needs a storage/retention review.** Already-created usage-cleanup tasks are not automatically cancelled.

Intentionally retained: normal account/key administration, backup creation/download/deletion, positive-period retention, and first-install initialization restart protected by the existing setup guard. This is not a globally read-only admin panel. The lower-level updater/restore implementations remain for upstream compatibility, with no enabled administrative Web/API entry point.

## Verification environment

- Go: exact `1.27.4`, matching `backend/go.mod`; locked modules, `-mod=readonly`.
- Frontend: Node `24.16.0`, pnpm `10.11.0`, frozen lockfile. Dependencies installed with lifecycle scripts disabled; no lockfile changes. The environment's default pnpm 12 rejected the old `pnpm.overrides` configuration; a compatible pnpm version was used instead of rewriting the lockfile.
- Tests/builds ran in separate user/network namespaces, with only namespace-local loopback and no external route. No existing localhost service or real model was used.
- Full backend tests also used a private mount-namespace hosts fixture mapping the two validation-only monitor domains to `192.0.2.1` (documentation address). The host's hosts file and proxy configuration were not changed. Proxy variables were unset only in test processes.
- Database validation used freshly initialized, disposable PostgreSQL `16.15` clusters, isolated network namespaces and synthetic data. PostgreSQL distribution packages were checksum-verified and unpacked without installing or starting a system service or using Docker.

## Results

| Check | Result |
|---|---|
| `bash scripts/calm/check.sh` | PASS: relevant service/repository checks, maintenance handler/route checks and Calm race tests |
| `go test -mod=readonly -p 3 -tags unit ./... -count=1 -timeout=300s` | PASS: 61 test-bearing packages, 50 packages without tests; isolated network, synthetic hosts fixture, process-local proxy cleanup |
| Frontend full Vitest | PASS: 388 files, 3149 tests |
| Frontend locale completeness, `vue-tsc -b`, Vite production build | PASS; existing large-chunk and Node deprecation warnings remain |
| Full backend build with `-tags embed` | PASS, explicit candidate version `0.2.22-calm.1-dev` and dirty-source identifier; binary was not run or deployed |
| Embedded web tests (`-tags 'unit embed'`) | PASS |
| Real PostgreSQL identity contract and race | PASS with the standard repository-package command: 3 top-level tests in each run, 0 skips, no race report; a fresh isolated cluster was used after fixing the test-constructor compile mismatch |
| Migration package and migration runner tests | PASS, including checksum rejection and transaction/nontransaction handling |
| Fresh database actual migrations | PASS: two concurrent `ApplyMigrations` calls, repeated application, 305 filenames/checksums verified |
| Synthetic upgrade with existing 303-file baseline and applied fork identity migration | PASS: both new `251_*` files applied; existing identities and NULL reservations preserved |
| Rollup trigger smoke tests | PASS on fresh and upgrade databases, including insert/update/delete and build invalidation |
| Existing migration integrity | PASS: original 303 SQL files unchanged; both new SQL files match upstream; fork `900001` matches the original identity checkpoint |
| Working-tree/index whitespace and conflict checks | PASS |

### Failures encountered and resolved

- Automatically merged upstream tests lacked the additional fork identity constructor argument or identity fixtures. These were corrected without weakening fail-closed behavior.
- Old tests expecting metadata passthrough preservation were updated to assert the explicit durable identity policy; API-key preservation cases remain covered.
- An old system-log UI test still expected a cleanup call from a now-disabled control; it now verifies no destructive call.
- In an initial full backend run, inherited proxies broke local fake CAPTCHA endpoints. A process-local proxy cleanup made the repository suite pass without code changes.
- Two upstream `TestValidateCreateParams_CheckModeMatrix` subtests attempted external DNS during validation. The same failure was reproduced on the unmodified upstream tag in the same disconnected environment. Namespace-local synthetic hosts allowed the full suite to run without external access or excluding those subtests. Ordinary opt-in integration skips remain; the PostgreSQL identity contract was run separately with its explicit disposable-database DSN.

## Migration and operational limits

The two `251_*` files are distinct migrations: the runner tracks complete filenames/checksums, not just numeric prefixes or a maximum version. Do not rename either file or alter already-applied SQL. The fork identity migration is byte-for-byte unchanged.

This review does **not** validate production topology, actual account identity import, production-scale locks/performance, all PostgreSQL versions, Docker image construction/import, live provider behavior, full container-based integration tests, browser E2E, or an application rollback rehearsal. A new migration's five-second lock timeout can still fail under real lock contention. Existing legacy NULL identities still require verified import, not automatic generation or deletion.

The inherited `backend/cmd/server/VERSION` remains `0.2.21` in the upstream tag. Candidate builds must pass an explicit fork version and record their dirty source state; do not infer release/deployment identity from that file or from a successful build. Deployment, database maintenance and restart require separate authorization and target-specific review.
