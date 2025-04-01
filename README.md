# VRChat Memes Bot

Telegram bot for posting VRChat memes to a channel.

## Features

- Posts memes to a specified Telegram channel
- Debug mode for development
- Error tracking with Sentry
- Environment-based configuration
- MongoDB integration for user tracking
- Docker support
- Hot-reload development mode with Air

## Requirements

- Go 1.24 or higher
- Telegram Bot Token
- Sentry DSN (for error tracking)
- MongoDB instance (optional, included in Docker setup)

## Installation

### Using Docker (Recommended)

1. Clone the repository:

    ```bash
    git clone https://github.com/yourusername/vrcmemes-bot.git
    cd vrcmemes-bot
    ```

2. Copy the example environment file and configure it:

    ```bash
    cp .env.example .env
    ```

3. Edit `.env` file with your configuration:

    ```env
    APP_ENV=development    # development, staging, or production
    DEBUG=true            # Enable debug mode
    VERSION=dev          # Application version
    USE_AIR=true         # Enable hot-reload in development mode

    TELEGRAM_BOT_TOKEN=your-bot-token
    CHANNEL_ID=your-channel-id
    SENTRY_DSN=your-sentry-dsn-here
    MONGODB_URI=mongodb://admin:password@mongodb:27017
    MONGODB_DATABASE=vrcmemes
    ```

4. Start the bot using Docker Compose:

    ```bash
    docker compose up -d
    ```

### Manual Installation

1. Clone the repository:

    ```bash
    git clone https://github.com/yourusername/vrcmemes-bot.git
    cd vrcmemes-bot
    ```

2. Install dependencies:

    ```bash
    go mod download
    ```

3. Copy the example environment file and configure it:

    ```bash
    cp .env.example .env
    ```

4. Edit `.env` file with your configuration:

    ```env
    APP_ENV=development    # development, staging, or production
    DEBUG=true            # Enable debug mode
    VERSION=dev          # Application version
    USE_AIR=false        # Disable hot-reload in production

    TELEGRAM_BOT_TOKEN=your-bot-token
    CHANNEL_ID=your-channel-id
    SENTRY_DSN=your-sentry-dsn-here
    MONGODB_URI=your-mongodb-uri
    MONGODB_DATABASE=your-database-name
    ```

## Environment Variables

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `APP_ENV` | Application environment (development/staging/production) | No | development |
| `DEBUG` | Enable debug mode | No | false |
| `VERSION` | Application version | Yes | - |
| `USE_AIR` | Enable hot-reload with Air in development mode | No | false |
| `TELEGRAM_BOT_TOKEN` | Your Telegram bot token | Yes | - |
| `CHANNEL_ID` | Telegram channel ID where memes will be posted | Yes | - |
| `SENTRY_DSN` | Sentry DSN for error tracking | Yes | - |
| `MONGODB_URI` | MongoDB connection URI | Yes | - |
| `MONGODB_DATABASE` | MongoDB database name | Yes | - |

## Development

### Using Docker

To run the bot in development mode with hot-reload:

```bash
# Set USE_AIR=true in .env file
docker compose up
```

To run the bot in production mode:

```bash
# Set USE_AIR=false in .env file
docker compose up
```

To stop the bot:

```bash
docker compose down
```

### Manual Development

To run the bot in development mode:

```bash
go run main.go
```

For hot-reload during development, you can use [Air](https://github.com/air-verse/air):

```bash
air
```

## Error Tracking

The bot uses Sentry for error tracking. Make sure to:

1. Create a Sentry account at https://sentry.io
2. Create a new project
3. Get your DSN from the project settings
4. Add the DSN to your `.env` file

## Database

The bot uses MongoDB to store:

- User information and activity
- Action logs
- Caption history

Make sure to:

1. Have a MongoDB instance running
2. Configure the connection URI in your `.env` file
3. Create a database for the bot

## Docker

The project includes Docker support with the following features:

- Multi-stage build for smaller final image
- Automatic MongoDB setup
- Volume persistence for database data
- Environment variable configuration
- Automatic container restart
- Hot-reload support in development mode

## License

MIT
