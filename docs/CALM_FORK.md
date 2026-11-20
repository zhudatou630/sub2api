# Calm fork maintenance

> This is an independently maintained fork of [Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api), not an official release. The modifications retain the upstream license in [`LICENSE`](../LICENSE); original attribution and notices remain intact.

## Branches and baseline

- `main`: upstream-derived branch; no local feature development here.
- `calm`: maintained fork branch.
- Current base: upstream release 0.2.22, commit `b0fa9f3e6eeab1968a53e1289b69daa4bb40ca2b`.
- Current source checkpoint: `calm-v0.2.22-identity.1` (application version `0.2.22-calm.1`).
- Previous base and checkpoint: upstream 0.2.21 `10ba8f9b4dc2e6023d4a9879ab77b21841babd10`, tag `calm-v0.2.21-identity.1` (application version `0.2.21-calm.1`).
- Upgrade candidate: upstream `v0.2.24`, commit `5c2982a81b46e2142d0143d2f83c1ce9c4da1d24`, integrated on `update/v0.2.24` (see [the review report](CALM_V0.2.24_REVIEW.md)). This is a source integration target, not a published fork release or evidence of deployment.
- Remotes: `origin` is this fork; `upstream` is the official repository.

The initial feature commit imports the already-reviewed identity patch without changing its source or tests. Later commits document maintenance. A source tag identifies code; it does not by itself publish or deploy a binary.

## Changes (2026-11-13)

1. Persist Anthropic OAuth/setup-token ClientIDs in PostgreSQL instead of treating Redis fingerprint entries as durable identities.
2. Derive UA/SDK fields from the server-owned `claude.DefaultHeaders()` bundle, retaining server-side CLI version synchronization. Inbound client headers cannot update that bundle.
3. Initialize new identities with an insert-if-absent operation followed by reading the committed winner. Database errors, invalid identities and reserved legacy identities fail closed.
4. Propagate identity errors through the affected Claude request/usage paths. Retain API-key and non-Claude behavior, routing, TLS and session derivation.
5. Redact identity/session and full-body diagnostic logs in the affected paths.

This is an identity-consistency change, **not a guarantee against account restrictions or a substitute for compliance with provider terms**.

## Existing installations: not a drop-in update

Migration `900001_calm_claude_identity.sql` reserves NULL identity rows for **all existing** Anthropic OAuth/setup-token accounts, including soft-deleted accounts. NULL deliberately means “verified import required”; it must not generate a replacement identity.

Before accepting traffic after migration:

1. Back up the database, cache, configuration and current identities privately.
2. Quiesce the old application and verify each current identity in the same trusted environment.
3. Import verified ClientIDs into their reserved rows transactionally, without logging or exporting values.
4. Check continuity and application health before resuming traffic.

This repository intentionally does not include installation-specific seed scripts, account lists, credentials, database dumps or production configuration. A fresh installation can initialize genuinely new accounts normally. Restored legacy NULL accounts require verified import; deleting identity rows is not a reset procedure.

Claude fingerprint Redis keys are no longer authoritative and may expire normally. Database availability is required. Directly reverting to an unpatched application can regenerate identities after legacy caches expire; rollback needs a current-identity migration plan, not restoration of old identity snapshots.

## Local verification

Use the exact Go version required by `backend/go.mod` (currently Go 1.27.4), without downgrading dependencies:

```bash
bash scripts/calm/check.sh
```

The script runs relevant unit and race tests with the opt-in PostgreSQL DSN unset. It does not connect to production, run containers, publish artifacts or deploy.

For the real PostgreSQL contract test, provision a **disposable local** database and explicitly set `CALM_IDENTITY_TEST_DSN`, then run:

```bash
cd backend
go test -mod=readonly ./internal/repository -run '^TestCalmClaudeIdentity' -count=1
```

The test requires an explicit loopback URL and creates/drops an isolated schema. Loopback is not proof a database is disposable: do not point it at a local production database, a production tunnel, or supply connection options that override the intended host. Synthetic accounts only; never use real tokens or conversation data.

Before a release, also compile the full application and run broader gateway/regression tests. At the initial checkpoint, relevant unit/race and real PostgreSQL concurrency tests passed. The full repository unit suite had the same pre-existing `TestAliyunCaptchaVerifier_TransportError` failure on both upstream and patched sources; it was not hidden or modified to claim a green full suite.

## Building

Use the upstream frontend/backend build process and Dockerfile. This source requires Go 1.27.4 and a Docker builder with BuildKit support. Build the frontend with its frozen lockfile before the backend's `embed` build, or build the binary with `-tags embed` against an already-built `internal/web/dist`. Pass an explicit fork version rather than relying on the inherited `cmd/server/VERSION` file (it can lag the upstream tag; `v0.2.24` still contains `0.2.23`). Without BuildKit, a verified alternative is to keep the matching official runtime image and replace only `/app/sub2api` with the locally built binary, then record both the runtime digest and the binary hash. A local, unpublished build can use:

```bash
docker build \
  --build-arg VERSION=0.2.22-calm.1 \
  --build-arg COMMIT="$(git rev-parse HEAD)" \
  -t sub2api-calm:local .
```

This builds locally only. A new build is not automatically byte-identical to an existing deployment: record source commit, toolchain, build inputs, binary hash and image identity, and verify the exported/imported artifact on target Docker versions. Production artifact storage and deployment are managed separately from this public repository.

## Following upstream

```bash
git fetch upstream --tags
git switch calm
git switch -c update/<reviewed-version>
git merge --no-ff <reviewed-upstream-tag-or-commit>
```

Review conflicts **and semantic changes**, especially identity consumers, constructors/Wire, migrations, default bundles and error handling. Run the tests above, full build and isolated migration/rollback checks. Prefer upstream's equivalent fix when suitable, but preserve existing identities when transitioning implementations.

After review, merge the update branch into `calm` and create a new annotated `calm-*` tag. Keep published history and already-applied migration contents stable; add a new migration for schema changes. Never force-push shared release history, silently overwrite a migration checksum, or deploy merely because a merge was clean.

## v0.2.22 hardening (tagged `calm-v0.2.22-identity.1`)

- Preserve PostgreSQL identity authority when legacy account identity fields disagree or `account_uuid` is absent. A missing account UUID is not permission to forward a downstream device/account identity.
- For Anthropic OAuth/setup-token, metadata passthrough no longer bypasses durable identity rewriting. The existing session derivation formula and optional masking remain unchanged, but enabling metadata passthrough no longer preserves the raw downstream session. API-key/non-Claude metadata behavior is retained.
- Apply the full server-owned default header bundle to native Claude and count_tokens requests. OAuth count_tokens mimicry skips downstream header passthrough like messages, and missing metadata is synthesized from the durable identity using existing derivation helpers.
- Administrative Claude connection/model probes also use the identity service and stop before sending when identity storage is unavailable, reserved or invalid. These are real-request features in normal use; regression tests use fake upstreams only.
- Concurrent token-rotation retries re-read identity and fail closed if it becomes invalid. Dependency error text is excluded from the affected retry/session logs.
- The two upstream `251_*` migrations are distinct filenames and both run even when the higher-numbered fork migration is already recorded. Do not rename them or modify already-applied SQL; the existing migration runner tracks full filenames/checksums, not a numeric high-water mark.

Validation and limitations are recorded in [the review report](CALM_V0.2.22_REVIEW.md). The tag identifies source only; it does not publish an image or deploy anything.

## Web/API maintenance restrictions

The fork disables in-process application updates, rollback/version selection, service restart, whole-database restore, audit-log clearing, creation of usage-cleanup tasks, and manual system-log cleanup. The backend rejects these operations with `403` / `DANGEROUS_MAINTENANCE_DISABLED`; hiding the frontend controls is not the security boundary. Version queries (including the legacy update-check endpoint) return local build metadata without GitHub/cache lookups. There is no administrator toggle to re-enable the operations.

Ops retention values of zero now **skip** cleanup instead of truncating a table, including existing persisted zero values; positive retention periods still expire old records. Review retention and storage needs when adopting this policy. Existing usage-cleanup tasks are not automatically cancelled; inspect and cancel them separately if needed.

Normal account/key administration, backup creation/download/deletion, and positive-period retention remain available. This is not a blanket read-only admin panel. First-install setup retains its initialization restart, guarded against use after installation; it is not the running instance's restart endpoint. Manual deployment remains a separately authorized operation. Backend updater/restore implementations are retained but must not be re-exposed by later merges.

## Publishing and repository boundaries

- `AGENTS.md` is intentionally local-only and ignored by Git. It may describe an operator's private workspace and deployment handoff, but must not contain credentials. A fresh clone does not include it; configure local operational instructions separately. Early revisions tracked a generic version: back up local instructions before checking out those revisions, or build them in a separate worktree to avoid overwriting an ignored file. This document remains the public, installation-independent maintenance guide.
- GitHub Actions are initially disabled at repository level. Inherited upstream workflows are retained for reference, not enabled as a release pipeline.
- Enabling CI, supplying registry credentials, publishing images or deploying requires a separately reviewed workflow. This fork has no automatic production deployment.
- No production `.env`, tokens, account identities, subscriptions, private URLs, infrastructure inventory, backups or logs belong here.
- Keep build outputs, source archives and Docker image archives out of Git. Operational records and deployment-specific configuration belong in a separate private operations workspace.
