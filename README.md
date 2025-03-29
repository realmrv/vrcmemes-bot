# Telegram Post Bot

A bot for creating posts in a Telegram channel.

## Installation

1. Clone the repository
2. Install dependencies:

```bash
go mod download
```

## Configuration

1. Create a bot through [@BotFather](https://t.me/botfather) in Telegram
2. Get the bot token
3. Copy the `.env.example` file to `.env`
4. Fill in the `.env` file:
   - `TELEGRAM_BOT_TOKEN` - your bot token
   - `CHANNEL_ID` - ID of the channel where posts will be published

## Running

```bash
go run main.go
```

## Usage

1. Send the `/start` command to the bot
2. Send the text you want to publish in the channel
3. The bot will create a post in the specified channel
