# Dasyra

[![CI](https://github.com/SergiuszN/dasyra/actions/workflows/ci.yml/badge.svg)](https://github.com/SergiuszN/dasyra/actions/workflows/ci.yml)
[![Go](https://img.shields.io/github/go-mod/go-version/SergiuszN/dasyra)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

**A personal manager that texts you first.**

Dasyra lives in your Telegram. She knows your goals for the year, remembers what you told her months ago, keeps your
day in order and checks in on her own, like a friend who actually cares whether you went to the gym.
Most AI agents do things for you. Dasyra helps you do your own.

<!-- TODO: 10-second GIF of a real chat where she writes first -->

Self-hosted: one small Go binary and a SQLite file. Usually under a dollar a month per person to run.

## What she does

- 🧭 **Keeps you on course.** On day one she asks for your "north stars", the few things that matter this year, and
  your priorities when they clash. Then she ties everyday plans back to them.
- 💬 **Writes first.** She checks in a few times a day and knows when to stay quiet: not right after you talked, and
  not again when you left her last two check-ins unanswered.
- 🧠 **Remembers.** One endless chat, no sessions to start over. She keeps durable facts, tidies her memory every
  night and finds things by meaning, in any language: ask in English about something you said in Polish.
- 📅 **Plans with you.** A to-do list for the day, a calendar with reminders at your local time, and regular jobs
  she runs herself: a morning digest, a weekly review, a price you want watched.
- 🔎 **Looks things up.** News, prices, opening hours, schedules: she searches the web through your own private
  search engine instead of guessing, and reads PDFs and pages for you.
- 🎙 **Talks like a person.** Short messages in your language, voice notes, photos and documents understood, a 👍
  instead of a reply when there is nothing to add.
- 📚 **Keeps your collections.** Recipes, gift ideas, films, places, packing lists: saved word for word and
  brought back when you need them.

## What she is not

Dasyra is deliberately narrow. She has **no shell, no access to your files, no plugins and no browser automation**.
She can't run commands on your server or reach your home network: web requests to internal addresses are refused.
The worst she can do is send you a bad message. That is the point: you can tell her about your whole life without
handing her your machine.

## Instead of a chatbot subscription

Dasyra isn't only a coach. Ask her anything you would ask ChatGPT: a recipe from what's in the fridge, a reply to an
awkward email, which of two laptops to buy, what that letter from the tax office means. The difference is that she
already knows you, remembers the answer next week and can remind you to act on it. One chat for everything, in the
messenger you already have open.

And she costs a fraction of a subscription. I carried this idea around for a long time, but until recently a model
smart enough to be a good companion was too expensive to run all day for a group of friends. Models like GPT-6 Luna
and Claude Haiku 5.5 changed that: really capable and cheap. Dasyra is built to make the most of it: prompts are
cached, memory is searched instead of stuffed into every request, and web pages are summarised by a small model before
she reads them. Even with heavy daily use it's hard to spend more than a couple of dollars a month per person, and
most months come in **under a dollar**, while a single chatbot subscription is around $20. The admin panel shows the
exact bill per person, per task and per model.

Use any model you like: OpenRouter by default, or any OpenAI-compatible API (OpenAI, DeepSeek, GLM, Groq), switchable
per role in the admin panel without a restart. Want your conversations to stay on your own hardware? A local Qwen 3.8
through Ollama handles her too.

## Your data stays yours

- Everything lives in one SQLite file on your server. Backing it up is one command.
- Messages go only to the model provider you choose.
- Web search runs on your own SearXNG instance, never exposed to the internet.
- Voice messages are transcribed and the audio is dropped.
- Invite-only: you decide who gets an account, and every user's data is kept apart.

## Get started

You need a server with Docker, a Telegram bot token from [@BotFather](https://t.me/BotFather), an
[OpenRouter](https://openrouter.ai) key and a public HTTPS address for the webhook (a Cloudflare Tunnel works).

```sh
git clone https://github.com/SergiuszN/dasyra && cd dasyra
cp .env.example .env        # fill in the keys and your Telegram chat id
docker compose -f compose.example.yml up -d
```

Point the bot's webhook at `https://<your host>/telegram/webhook` with your `TELEGRAM_WEBHOOK_SECRET`, say hi to
your bot, and Dasyra starts getting to know you. Invite friends with
`docker compose -f compose.example.yml exec dasyra dasyra invite`.

## Contributing

Ideas, bug reports and pull requests are welcome. The scope is narrow on purpose: Dasyra gets better at being a
good manager and friend, not at doing more things. If you plan something big, open an issue first.

```sh
make up      # local search engine
make run     # the bot with long polling, keys from .env
make test
```

## License

[MIT](LICENSE): use it, change it, run it for your friends or your company.
