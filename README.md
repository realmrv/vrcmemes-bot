# VRChat Memes Bot

Telegram bot for posting VRChat memes to a channel, featuring a suggestion and review system.

## Features

- Posts memes directly to a specified Telegram channel (Admin only)
- **User Suggestion System:** Allows channel subscribers to suggest posts (/suggest).
- **Admin Review Queue:** Admins can review (/review), approve, or reject suggestions.
- **Localization:** Supports multiple languages (EN, RU) using `go-i18n`.
- Debug mode for development.
- Error tracking with Sentry (including panic recovery).
- Enhanced error handling with context wrapping.
- Improved logging with contextual information.
- Detailed GoDoc comments throughout the codebase.
- Robust media group handling with rate limit retries.
- Environment-based configuration.
- MongoDB integration for user tracking, post logging, and suggestion storage.
- Docker support (Development & Production environments).
- Hot-reload development mode with Air.

## Requirements

- Go 1.24 or higher
- Docker & Docker Compose
- Telegram Bot Token
- Sentry DSN (for error tracking)
- Key dependencies:
  - `github.com/mymmrac/telego v1.0.2`
  - `github.com/getsentry/sentry-go v0.31.1`
  - `go.mongodb.org/mongo-driver v1.17.3`
  - `github.com/joho/godotenv v1.5.1` (for loading `.env` files)
  - `github.com/nicksnyder/go-i18n/v2 v2.4.0`

## Installation & Running with Docker (Recommended)

1. **Clone the repository:**

    ```bash
    git clone https://github.com/yourusername/vrcmemes-bot.git
    cd vrcmemes-bot
    ```

2. **Copy the example environment file and configure it:**

    ```bash
    cp .env.example .env
    ```

3. **Edit `.env` file with your configuration:**
    - Set `TELEGRAM_BOT_TOKEN`, `CHANNEL_ID`, `SENTRY_DSN`.
    - Adjust `MONGO_INITDB_ROOT_USERNAME` and `MONGO_INITDB_ROOT_PASSWORD` if needed (defaults are 'admin'/'password').
    - Other variables like `APP_ENV`, `DEBUG`, `VERSION` can be configured as needed.
    - **Note:** The `MONGODB_URI` is automatically configured for Docker Compose. For manual runs, you'll need to set it appropriately.

    ```env
    # Example .env
    APP_ENV=development    # development, staging, or production
    DEBUG=true            # Enable debug mode
    VERSION=dev          # Application version

    TELEGRAM_BOT_TOKEN=your-bot-token
    CHANNEL_ID=your-channel-id # Bot will check admin status here
    SENTRY_DSN=your-sentry-dsn-here

    # MongoDB Credentials (used by docker-compose.yml)
    MONGO_INITDB_ROOT_USERNAME=admin
    MONGO_INITDB_ROOT_PASSWORD=password

    # MongoDB Connection (usually set for non-docker runs or specific overrides)
    # MONGODB_URI=mongodb://admin:password@localhost:27017
    MONGODB_DATABASE=vrcmemes
    ```

4. **Run in Development Mode (with Hot-Reload):**
    This uses `docker-compose.yml` and `docker-compose.override.yml` to run the `builder` stage with `air` and mounts your local code.

    ```bash
    docker compose up --build
    ```

    *(Add `-d` to run in the background)*

5. **Run in Production Mode:**
    This uses *only* `docker-compose.yml` to run the final, optimized stage. It builds the production image and does not mount local code.

    ```bash
    docker compose -f docker-compose.yml up --build -d
    ```

6. **Stopping the Application:**

    ```bash
    # If started with 'docker compose up' (dev mode)
    docker compose down

    # If started with 'docker compose -f docker-compose.yml up' (prod mode)
    docker compose -f docker-compose.yml down
    ```

## Manual Installation & Running (Not Recommended for Production)

1. Clone, install dependencies (`go mod download`), and configure `.env` as described above. Ensure `MONGODB_URI` points to your accessible MongoDB instance.
2. Run the application directly:

    ```bash
    go run main.go
    ```

3. For hot-reload during manual development:

    ```bash
    # Make sure air is installed (go install github.com/air-verse/air@latest)
    air
    ```

## Environment Variables

| Variable                       | Description                                              | Required             | Default         |
| ------------------------------ | -------------------------------------------------------- | -------------------- | --------------- |
| `APP_ENV`                      | Application environment (development/staging/production) | No                   | `development`   |
| `DEBUG`                        | Enable debug mode                                        | No                   | `false`         |
| `VERSION`                      | Application version                                      | Yes                  | -               |
| `TELEGRAM_BOT_TOKEN`           | Your Telegram bot token                                  | Yes                  | -               |
| `CHANNEL_ID`                   | Telegram channel ID where memes will be posted and admin status checked | Yes                  | -               |
| `SENTRY_DSN`                   | Sentry DSN for error tracking                            | Yes                  | -               |
| `MONGO_INITDB_ROOT_USERNAME` | MongoDB root username for initialization               | No (used by Docker)  | `admin`         |
| `MONGO_INITDB_ROOT_PASSWORD` | MongoDB root password for initialization               | No (used by Docker)  | `password`      |
| `MONGODB_URI`                  | MongoDB connection URI                                   | Yes (for manual run) | -               |
| `MONGODB_DATABASE`             | MongoDB database name                                    | Yes                  | -               |

## User Roles & Admin Check

- **Admin:** Determined by having `creator` or `administrator` status in the Telegram channel specified by `CHANNEL_ID`. Admins can use all bot commands *except* `/suggest`. They can post directly, manage captions, and review suggestions (`/review`).
- **User/Subscriber:** Can use `/start`, `/help`, and `/suggest` to propose posts. Must be subscribed to the target channel (`CHANNEL_ID`) to use `/suggest`.

## Commands

### User Commands

- `/start`: Start interaction with the bot and get a welcome message.
- `/help`: Show help information.
- `/suggest`: Start the process of suggesting a post for the channel. (Requires channel subscription)

### Admin Commands

- `/start`: Start interaction with the bot and get a welcome message.
- `/help`: Show help information.
- `/status`: Show bot status and current caption.
- `/version`: Show bot version.
- `/review`: Start reviewing pending suggestions.
- (Direct messages): Send photos, videos, or media groups directly to the bot to post them to the channel. Text in the message will be used as the caption.

## Suggestion Workflow

1. A user subscribes to the channel defined by `CHANNEL_ID`.
2. The user sends `/suggest` to the bot.
3. The bot asks the user to send a photo or a media group (up to 10 photos).
4. The user sends the media. The message text is stored as a private comment for the suggestion.
5. The bot confirms receipt and stores the suggestion (including media file IDs and comment) in MongoDB with "pending" status.
6. An admin (checked via channel status) uses `/review`.
7. The bot presents the oldest pending suggestion (media + comment + submitter info) with Approve/Reject/Next buttons.
8. Admin approves: The bot posts the media to the channel (using the suggestion's comment as caption if desired, or a default caption) and updates the suggestion status to "approved".
9. Admin rejects: The bot updates the suggestion status to "rejected".
10. Admin skips (Next): The bot shows the next pending suggestion.

## Localization

The bot uses `github.com/nicksnyder/go-i18n/v2` for localization. Language files (`en.json`, `ru.json`) are located in `pkg/locales/`. The default language is set in `pkg/locales/i18n.go`.

## Project Structure

```
.
├── bot/                     # Core bot logic, Telegram API interaction, update loop
├── config/                  # Configuration loading (.env)
├── database/                # MongoDB interaction (connection, models, repository interfaces)
├── handlers/                # Telegram message/command handlers (routing, initial processing)
├── internal/
│   └── suggestions/       # Logic for suggestion handling, review process, admin checks
├── pkg/
│   └── locales/           # Localization files (en.json, ru.json) and i18n setup
├── .air.toml                # Air configuration for hot-reload
├── .env.example             # Example environment variables
├── .gitignore               # Git ignore rules
├── Dockerfile               # Docker build instructions (multi-stage)
├── README.md                # This file
├── docker-compose.yml       # Docker Compose setup (Production base)
├── docker-compose.override.yml # Docker Compose overrides (Development)
├── go.mod                   # Go module dependencies
├── go.sum                   # Go module checksums
└── main.go                  # Application entry point
```

## Development

See the **Installation & Running with Docker** section for running in development mode using Docker Compose, which is the recommended approach.

Manual development using `go run` or `air` is possible but requires manual setup of dependencies like MongoDB.

## Error Tracking

The bot uses Sentry for error tracking. Ensure `SENTRY_DSN` is set in your `.env` file.

## Database

The bot uses MongoDB to store user actions, published post logs, and suggestions. The Docker Compose setup includes a MongoDB service. Database credentials (`MONGO_INITDB_ROOT_USERNAME`, `MONGO_INITDB_ROOT_PASSWORD`) and the database name (`MONGODB_DATABASE`) are configured via `.env`.

## Docker Details

- **Multi-stage Build:** `Dockerfile` uses a builder stage for dependencies/compilation and a minimal final stage for the production image.
- **Platform Aware:** The build automatically detects the target architecture (`amd64`, `arm64`).
- **Development Environment:** `docker compose up` uses `docker-compose.override.yml` to run the `builder` stage, mounts local code, and uses `air` for hot-reloading.
- **Production Environment:** `docker compose -f docker-compose.yml up` uses only the base configuration, builds the final lean image, and runs the compiled binary.
- **MongoDB Service:** Included in Docker Compose for convenience.
- **Configuration:** Environment variables are loaded from the `.env` file.

## CI/CD

This project uses GitHub Actions for basic Continuous Deployment.

- **Workflow:** `.github/workflows/deploy.yml`
- **Trigger:** Pushes to the `develop` branch.
- **Action:** Connects to the production server via SSH, pulls the latest changes from the `develop` branch, and rebuilds/restarts the Docker containers using `docker-compose up -d --build`.

### Required Secrets

For the deployment workflow to function, the following secrets must be configured in the GitHub repository settings (`Settings` > `Secrets and variables` > `Actions`):

- `SSH_PRIVATE_KEY`: Private SSH key to access the server.
- `SSH_HOST`: Hostname or IP address of the server.
- `SSH_USER`: Username for SSH connection.
- `SERVER_PROJECT_PATH`: Absolute path to the project directory on the server.

## License

MIT
