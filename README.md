# tgcli

> Telegram from your terminal — as **you**, not a bot.

<p>
  <a href="https://github.com/kaosb/tgcli/releases"><img src="https://img.shields.io/github/v/release/kaosb/tgcli?style=flat-square&color=blue" alt="Release"></a>
  <a href="https://github.com/kaosb/tgcli/blob/main/LICENSE"><img src="https://img.shields.io/github/license/kaosb/tgcli?style=flat-square" alt="License"></a>
  <img src="https://img.shields.io/github/go-mod/go-version/kaosb/tgcli?style=flat-square" alt="Go version">
</p>

tgcli is a command-line interface for Telegram that uses your **personal account** via MTProto 2.0 — the same protocol used by official Telegram apps. Send messages, search conversations, sync history, and download media, all from your terminal.

Inspired by [wacli](https://github.com/steipete/wacli) (WhatsApp CLI).

## Why tgcli?

| Tool | Operates as you | Send messages | Local search | Maintained |
|---|:---:|:---:|:---:|:---:|
| **tgcli** | **Yes** | **Yes** | **Yes** | **Yes** |
| [tdl](https://github.com/iyear/tdl) | Yes | No | No | Yes |
| [telegram-send](https://github.com/rahiel/telegram-send) | No (bot) | Yes | No | No |
| [telegram-cli](https://github.com/vysheng/tg) | Yes | Yes | No | No (2016) |

## Quick start

```bash
# 1. Install
CGO_ENABLED=1 go install github.com/kaosb/tgcli@latest

# 2. Login (once — you'll be prompted for API credentials and phone)
tgcli login

# 3. Use it
tgcli chat ls
tgcli send text @username "Hello from the terminal!"
tgcli msg search "meeting notes"
```

## Install

### From source (requires Go 1.22+ and a C compiler)

```bash
git clone https://github.com/kaosb/tgcli.git && cd tgcli
make build
# Binary at ./tgcli
```

### With `go install`

```bash
CGO_ENABLED=1 go install github.com/kaosb/tgcli@latest
```

Make sure `$GOPATH/bin` is in your `PATH`:

```bash
# Add to ~/.zshrc or ~/.bashrc
export PATH="$HOME/go/bin:$PATH"
```

### From releases

Download the latest binary from [GitHub Releases](https://github.com/kaosb/tgcli/releases).

> **Note:** tgcli uses SQLite via CGO (`mattn/go-sqlite3`), so `CGO_ENABLED=1` is required at build time. Pre-built binaries don't need this.

## Setup

tgcli needs a Telegram API application. Create one at [my.telegram.org/apps](https://my.telegram.org/apps) (any name works) — you'll need the `api_id` and `api_hash`.

Then just run:

```bash
tgcli login
```

The login flow is fully interactive and guides you through each step:

1. **API credentials** — Enter your `app_id` and `app_hash` (saved automatically for next time)
2. **Phone number** — Your number with country code (e.g. `+56912345678`)
3. **Verification code** — Telegram sends a code to your app (or via SMS)
4. **2FA password** — Only if you have two-factor authentication enabled

```
$ tgcli login
Telegram API credentials required (get them at https://my.telegram.org/apps)
App ID: 12345678
App Hash: your_api_hash
Credentials saved to /Users/you/.tgcli/config.json
Phone number (with country code, e.g. +56...): +56912345678
Verification code (check Telegram): 12345
2FA password: ********
Logged in as John Doe (ID: 123456789)
```

The session persists — you only need to do this once. Credentials are stored in `~/.tgcli/config.json` and the session in `~/.tgcli/session.json`.

> **Alternative:** You can also set credentials via environment variables `TGCLI_APP_ID` and `TGCLI_APP_HASH` (these take priority over the config file).

## Commands

### Logout

```bash
tgcli logout                                # End session and remove local session
```

### Send messages

```bash
tgcli send text @username "Hello!"          # By username
tgcli send text 123456789 "Hello!"          # By user ID
tgcli send file @username ./report.pdf      # Send a file
tgcli send file @username ./photo.jpg --caption "Check this out"
```

### List chats

```bash
tgcli chat ls                               # All chats
tgcli chat ls --type private                # Only DMs
tgcli chat ls --type group --limit 20       # Groups
```

### Read messages

```bash
tgcli msg ls @username                      # Last 20 messages
tgcli msg ls @username --limit 50           # Last 50
tgcli msg context @username 12345           # Messages around #12345
```

### Search

Search uses SQLite FTS5 for instant offline full-text search. Run `tgcli sync` first to build the local index.

```bash
tgcli msg search "meeting notes"            # Search all chats
tgcli msg search "budget" --chat @username  # Search one chat
```

### Sync history

```bash
tgcli sync                                  # All chats (recent messages)
tgcli sync --chat @username                 # Full history for one chat
```

### Export

```bash
tgcli export @username                      # From Telegram API (JSON)
tgcli export @username --local              # From local DB (no connection needed)
tgcli export @username -o backup.json       # Save to file
```

### Download media

```bash
tgcli download @username 12345              # Download media from message
tgcli download @username 12345 -o ~/media   # Custom output directory
```

## Chat identifiers

You can refer to chats in multiple ways:

| Format | Example | Type |
|---|---|---|
| `@username` | `@durov` | Username |
| `123456789` | `123456789` | User ID |
| `-123456789` | `-123456789` | Group |
| `-100123456789` | `-100123456789` | Channel / Supergroup |
| `+1234567890` | `+56912345678` | Phone (must be in contacts) |

## Global flags

Every command supports these:

| Flag | Description | Default |
|---|---|---|
| `--json` | Machine-readable JSON output | `false` |
| `--store DIR` | Data directory | `~/.tgcli` |
| `--timeout DUR` | Command timeout | `5m` |

## How it works

tgcli connects to Telegram through [gotd/td](https://github.com/gotd/td), a pure-Go MTProto 2.0 implementation. This means:

- **You are the sender** — messages come from your account, not a bot
- **No ban risk** — Telegram officially supports third-party clients
- **Independent session** — works without your phone being online
- **Login once** — session persists until you revoke it

Messages fetched from the API are cached in a local SQLite database with FTS5 full-text search, enabling instant offline search across your entire history.

## Data storage

All data lives in `~/.tgcli/` (configurable with `--store`):

| File | What it is |
|---|---|
| `config.json` | API credentials (`app_id`, `app_hash`) — **keep this private** |
| `session.json` | MTProto session — **keep this private** |
| `tgcli.db` | Local message cache + FTS5 search index |

## Non-interactive login

For CI/CD or scripting, you can pass each credential as a flag. Flags and interactive prompts can be mixed — any flag you provide skips its corresponding prompt:

```bash
# Provide phone upfront, enter code interactively
tgcli login --phone "+1234567890"

# Fully non-interactive (e.g. for scripts)
tgcli login --phone "+1234567890" --code "12345" --password "your2fa"
```

> **Note:** The `--code` flag requires you to have already triggered a code (e.g. from a prior `tgcli login` attempt). For most users, the interactive flow is simpler.

## Tech stack

- [gotd/td](https://github.com/gotd/td) — MTProto 2.0 protocol (pure Go)
- [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3) — SQLite with FTS5
- [spf13/cobra](https://github.com/spf13/cobra) — CLI framework

## Acknowledgments

- [wacli](https://github.com/steipete/wacli) — WhatsApp CLI that inspired this project's design
- [tdl](https://github.com/iyear/tdl) — Telegram downloader that proved the gotd/td approach
- [gotd/td](https://github.com/gotd/td) — the excellent Go MTProto library that makes this possible

## License

[AGPL-3.0](LICENSE)
