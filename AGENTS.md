# Maintained fork constraints

- Answer in Chinese unless the user requests otherwise.
- Read `docs/CALM_FORK.md` before changing the maintained identity implementation.
- `calm` is the maintained branch; `upstream` is official upstream. Review updates on a separate branch; do not force-push published history or overwrite release tags.
- Keep the fork small. Preserve API-key/non-Claude behavior, account credentials, routing, TLS and session derivation unless explicitly authorized to change them.
- Existing identity values must remain stable. Database errors and reserved NULL identities must fail closed, not generate replacement IDs.
- Never change the contents/checksum of an already-applied SQL migration. Add a new migration for later schema changes.
- This is a public source repository. Never commit production credentials, account identities, private URLs, installation configuration, backups, logs or image archives. `.gitignore` is not a substitute for reviewing the staged diff.
- Tests use synthetic data. PostgreSQL integration tests require an explicitly chosen disposable local database; a loopback address alone does not prove a database is safe to modify. Never use production tunnels or real model credentials.
- Source maintenance does not authorize deployment, server access, service restarts, enabling release workflows or publishing images. Those are separate operations requiring explicit authorization.
- Preserve upstream licensing and attribution. Do not present this fork as an official release or promise that identity consistency prevents provider account restrictions.
