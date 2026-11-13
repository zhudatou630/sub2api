# Calm fork maintenance

> This is an independently maintained fork of [Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api), not an official release. The modifications retain the upstream license in [`LICENSE`](../LICENSE); original attribution and notices remain intact.

## Branches and baseline

- `main`: upstream-derived branch; no local feature development here.
- `calm`: maintained fork branch.
- Initial base: upstream commit `10ba8f9b4dc2e6023d4a9879ab77b21841babd10`, release 0.2.21.
- Initial source checkpoint: `calm-v0.2.21-identity.1` (application version `0.2.21-calm.1`).
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

Use the exact Go version required by `backend/go.mod` (initially Go 1.27.3), without downgrading dependencies:

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

Use the upstream frontend/backend build process and Dockerfile. The initial source requires Go 1.27.3 and a Docker builder with BuildKit support. Build the frontend with its frozen lockfile before the backend's `embed` build. Pass an explicit fork version rather than relying on the inherited `cmd/server/VERSION` file:

```bash
docker build \
  --build-arg VERSION=0.2.21-calm.1 \
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

## Publishing and repository boundaries

- GitHub Actions are initially disabled at repository level. Inherited upstream workflows are retained for reference, not enabled as a release pipeline.
- Enabling CI, supplying registry credentials, publishing images or deploying requires a separately reviewed workflow. This fork has no automatic production deployment.
- No production `.env`, tokens, account identities, subscriptions, private URLs, infrastructure inventory, backups or logs belong here.
- Keep build outputs, source archives and Docker image archives out of Git. Operational records and deployment-specific configuration belong in a separate private operations workspace.
