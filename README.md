# transcribbot

Telegram bot to transcribe audios.

## Setup

Download a [Vosk-API model](https://alphacephei.com/vosk/models). Unzip it and
set `VOSK_MODEL_PATH` pointing to its directory (by default, it is
`$(pwd)/model`).

Ask [BotFather](https://t.me/BotFather) for a token and set
`TELEGRAM_BOT_TOKEN`.

Install `ffmpeg`.

Run `main.go`.
