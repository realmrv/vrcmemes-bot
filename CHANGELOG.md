# Changelog

## v0.1.0 - 2026-09-27

First tagged release.

- Update Go to 1.27.1 and maintained direct dependencies.
- Pin Go, Alpine, and MongoDB container images to versioned multi-architecture digests; keep MongoDB on the existing 8.3 series.
- Require authenticated MongoDB readiness and remove public example credential defaults.
- Exit when Telegram polling closes unexpectedly and bound concurrent update processing.
- Include the dependency-container refactor with startup cleanup and repair the `sync.Map` test copy.
- Separate CI from manual deployment of a published release tag, verify the SSH host key, and preserve the previous bot image for application-only rollback.
- Document local setup, Ubuntu 26 recovery, credential rotation, health checks, and rollback.

Production operators must back up the existing MongoDB volume, verify provider console access, recover a compatible kernel, and rotate the persisted MongoDB password before deploying this release. Updating `MONGO_INITDB_ROOT_PASSWORD` alone does not rotate an existing user.
