# tg-ringer (Go)

Ring (call) and message any Telegram user from **your own account** — a
[gotd/td](https://github.com/gotd/td) userbot for urgent alerts. Placing a private
Telegram call makes the target's phone **ring** (no audio — the ring is the alert),
then hangs up. Go port of [tg-ringer](https://github.com/jdp5949/tg-ringer) (Python).

> Userbot = Telegram **ToS gray area**; accounts (esp. VoIP numbers) can be banned.
> Use a throwaway account, mutual contacts, low volume. See the Python project for
> full notes. **Call feature is beta in this port** (mirrors the verified Python logic).

## Install

**Download a prebuilt binary** from [Releases](https://github.com/jdp5949/tg-ringer-go/releases)
(Linux/macOS/Windows, amd64/arm64), make it executable, and put it on your `PATH`:

```bash
# example: macOS arm64
curl -L -o tg-ringer https://github.com/jdp5949/tg-ringer-go/releases/latest/download/tg-ringer-darwin-arm64
chmod +x tg-ringer && sudo mv tg-ringer /usr/local/bin/
```

Or with Go:

```bash
go install github.com/jdp5949/tg-ringer-go/cmd/tg-ringer@latest
```

## Usage

```bash
tg-ringer login                # interactive setup (api_id/api_hash) + sign in
tg-ringer call +15551234567    # ring a number
tg-ringer call @user 30        # ring for 30s
tg-ringer msg  +15551234567 deploy finished
tg-ringer whoami
tg-ringer config
```

Get `api_id` / `api_hash` at <https://my.telegram.org>. The login code arrives
**inside Telegram** (the "Telegram" service chat), not SMS. Use a **separate**
account as the userbot — you can't call yourself.

Config is saved to `~/.config/tg-ringer-go/config` (chmod 600); env vars
(`TG_API_ID`, `TG_API_HASH`, `TG_TARGET`, `RING_SECONDS`, `TG_SESSION`) override.

## Library

```go
import "github.com/jdp5949/tg-ringer-go/ringer"

ringer.Run(ctx, cfg, func(ctx context.Context, c *ringer.Client) error {
    _, err := c.Ring(ctx, "+15551234567", 20)
    return err
})
```

## License

MIT © jdp5949
