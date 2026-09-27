# VRChat Memes Bot

A Telegram bot for posting VRChat memes to a channel, collecting suggestions, and reviewing them with channel administrators.

## Requirements

- Go 1.27.1 for local builds, or Docker with Compose for container builds.
- A bot token from [BotFather](https://t.me/BotFather), the numeric channel ID, and administrator access for the bot in that channel.
- A MongoDB password generated for this installation. `openssl rand -hex 32` produces a password that can be placed in a MongoDB URI without extra encoding.

The maintained dependency versions are recorded in `go.mod`. Sentry is optional.

## Quickstart

```sh
git clone https://github.com/realmrv/vrcmemes-bot.git
cd vrcmemes-bot
cp .env.example .env
chmod 600 .env
```

Edit `.env` and set `TELEGRAM_BOT_TOKEN`, `CHANNEL_ID`, `MONGO_INITDB_ROOT_PASSWORD`, and `MONGODB_URI`. For example, when the username is `admin` and the generated password is `YOUR_GENERATED_PASSWORD`, the URI is:

```text
mongodb://admin:YOUR_GENERATED_PASSWORD@mongodb:27017/?authSource=admin
```

Use the same password in `MONGO_INITDB_ROOT_PASSWORD` and the URI. If a password contains reserved URI characters, [percent-encode](https://www.mongodb.com/docs/manual/reference/connection-string/) them in `MONGODB_URI`. Leave `SENTRY_DSN` empty to disable Sentry. Never commit `.env`.

Start the production configuration explicitly:

```sh
docker compose -f docker-compose.yml config --quiet
docker compose -f docker-compose.yml up -d --build --wait --wait-timeout 120
docker compose -f docker-compose.yml ps
```

MongoDB should report `healthy`, and the bot should be `running`. A channel administrator can then send `/status` to the bot in a private chat and confirm a reply. The bot does not post this reply to the channel. This is the final user-visible check; container status alone does not prove that Telegram polling works.

For local hot reload, first copy `docker-compose.override.example.yml` to the ignored `docker-compose.override.yml`, then run `docker compose up --build`. The default Compose command loads that development override. Production commands always specify `-f docker-compose.yml` so they do not load it.

## Configuration

| Variable | Purpose |
|---|---|
| `TELEGRAM_BOT_TOKEN` | Required BotFather token |
| `CHANNEL_ID` | Required numeric channel ID used for posting and administrator checks |
| `MONGODB_URI` | Required runtime connection URI used by the bot |
| `MONGODB_DATABASE` | Required application database name; `vrcmemes` in the example |
| `MONGO_INITDB_ROOT_USERNAME` | Root user created only when a MongoDB volume is initialized |
| `MONGO_INITDB_ROOT_PASSWORD` | Root password created only when a MongoDB volume is initialized; also used by the authenticated health check |
| `VERSION` | Bot version; the release workflow sets this to the deployed tag |
| `APP_ENV`, `DEBUG`, `BOT_DEFAULT_LANGUAGE`, `SENTRY_DSN` | Optional runtime settings |

Changing `MONGO_INITDB_ROOT_USERNAME` or `MONGO_INITDB_ROOT_PASSWORD` in `.env` does **not** change an existing user in a persistent MongoDB volume. Rotate that user inside MongoDB first, then update both the health-check credentials and the bot's `MONGODB_URI`. Keep the `mongodb_data` named volume intact.

## Commands

Users can run `/start`, `/help`, `/suggest`, and `/feedback`. Channel administrators can also run `/status`, `/version`, `/caption`, `/showcaption`, `/clearcaption`, and `/review`. Administrators can send photos, videos, or media groups to the bot for direct posting. Suggestions are stored in MongoDB and can be approved or rejected from the review queue.

## Release deployment

Pushes to `develop` run Go tests, vet, Compose validation, and an amd64 image build. They do not deploy. Publish a GitHub release such as `v0.1.1` from the tested commit, then run **Actions → Verify and deploy release → Run workflow** with the required `release_tag`. The workflow rejects missing, draft, prerelease, and malformed tags. It verifies the server host key, deploys the tag's exact commit, and reports the commit SHA, bot image ID, and database/bot health in its summary.

Production Actions secrets are `SSH_PRIVATE_KEY`, `SSH_HOST`, `SSH_PORT`, `SSH_USER`, and `SERVER_PROJECT_PATH`. The host key fingerprint embedded in the workflow must be verified independently in the provider console before the first deployment or after a legitimate host-key change. The server checkout must be clean. Keep the server's `.env` outside Git and permission restricted.

### Ubuntu 26 recovery

If MongoDB logs report a Linux kernel incompatibility, check `uname -r` and `docker logs --tail 80 vrcmemes-mongodb`. MongoDB 8.3.11 on this host rejected kernel `7.0.0-34-generic` after the Ubuntu 26.04 upgrade. This Compose file sets `MONGO_TCMALLOC_PER_CPU_CACHE_SIZE_BYTES=0` and clears the image's `GLIBC_TUNABLES` value. Together they disable the TCMalloc per-CPU cache path that triggers MongoDB's startup guard. The pinned MongoDB 8.3.11 image was tested on this host's x86_64 kernel with authenticated reads and writes, and no restart during the smoke window. Keep a verified backup of `vrcmemes-bot_mongodb_data` and `.env` before recreating the database container. Do not change the host kernel or downgrade the database for this release. This is a temporary, locally verified workaround; [MongoDB's supported remedy](https://www.mongodb.com/docs/manual/administration/production-notes/) is a compatible kernel. Disabling the cache may reduce allocator throughput, so monitor the bot and database after deployment. Recheck for a compatible maintained kernel by 2026-10-27, then remove the workaround only after a separate stability test without it.

### Deployment troubleshooting

| Failure | Likely cause | Next check |
|---|---|---|
| Compose reports missing MongoDB variables | `.env` lacks the required root values | Set both root variables and `MONGODB_URI`; run `docker compose -f docker-compose.yml config --quiet`. |
| SSH fingerprint mismatch | Server identity changed or key was replaced | Stop deployment; compare the ED25519 fingerprint in the provider console with the workflow value. |
| MongoDB is unhealthy | Missing TCMalloc workaround, wrong password, or storage issue | Run `uname -r` and `docker logs --tail 80 vrcmemes-mongodb`; verify both Compose environment settings and an authenticated database operation. |
| Bot stops or restarts | Runtime URI, token, or Telegram polling error | Run `docker logs --tail 80 vrcmemes-bot-prod` and inspect `.env` without printing secrets. |
| Workflow tag validation fails | Tag is absent, unpublished, draft, or prerelease | Publish the release, then rerun the workflow with the exact `vX.Y.Z` tag. |

### Rollback

The deploy workflow tags the previous bot image as `vrcmemes-bot:rollback` before building. To restore **only** the bot, create a temporary override on the server:

```sh
cat > /tmp/vrcmemes-bot-rollback.yml <<'EOF'
services:
  bot:
    image: vrcmemes-bot:rollback
EOF
docker compose -f docker-compose.yml -f /tmp/vrcmemes-bot-rollback.yml up -d --no-deps --no-build bot
docker compose -f docker-compose.yml ps
```

This leaves the pinned MongoDB image and named volume untouched. Verify the bot logs and an administrator's `/status` reply. Never run `docker compose down -v` against the production stack.

## Development

Run `go test ./...`, `go vet ./...`, and `go mod verify` before submitting changes. Run `go run .` after setting the required environment variables when developing without Docker.

## License

MIT
