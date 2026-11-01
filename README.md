# ResonBot

XMPP bots for exercising ResonChat features. Built on a local mellium fork
(`vendor/mellium_xmpp`, `go.mod` `replace`).

Joins one or more MUCs. Optional features (each toggled with `enabled` in TOML):

* **translate** — plaintext [Peer Message Translation](../docs/12-peertrans.md) replies
* **antispam** — rate / repeat / nick rules → warn or ban (`affiliation=outcast`)
* **retract** — optional XEP-0425 moderate IQ after spam (mellium `moderation`)

### Config

```bash
cp config.example.toml config.toml
# edit config.toml
go run -mod=mod ./cmd -config config.toml
```

```toml
[xmpp]
jid = "bot@example.org"
password = "secret"

[[rooms]]
jid = "room@muc.example"
nick = "resonbot"

[translate]
enabled = true
provider = "openai"          # deepl | openai | anthropic
api_key = "sk-…"
# api_url = "…"              # optional endpoint override
model = "gpt-4o-mini"
# one target or a list (parallel API calls per message)
to = ["zh-CN", "ja", "en"]
from = "auto"
priority = 3

[antispam]
enabled = false

[retract]
enabled = false

[log]
verbose = true
xml = false
```

### Translation (kiss-translator aligned)

| `translate.provider` | API shape | Default URL |
| --- | --- | --- |
| `deepl` | DeepL official `v2/translate` | `https://api-free.deepl.com/v2/translate` |
| `openai` | OpenAI Chat Completions | `https://api.openai.com/v1/chat/completions` |
| `anthropic` | Anthropic Messages | `https://api.anthropic.com/v1/messages` |

`openai` covers OpenAI-compatible gateways (DeepSeek, OpenRouter, Ollama,
Gemini OpenAI mode, …) via `translate.api_url`. LLM prompts match
kiss-translator’s **concise nobatch** preset.

`translate.to` accepts a single string or a TOML array. Each inbound message is
translated to every listed language in parallel; targets whose primary tag
already matches the detected source are skipped.

### Resilience (Conversations-aligned)

* Auto-reconnect with `getTimeToNextAttempt` backoff (same formula as
  Conversations / ResonChat).
* XEP-0198 Stream Management via mellium `sm`.

### Check in ResonChat

1. Join the same room with a supporting client (trust everyone, or make the bot
   a MUC admin if “admins only” is on).
2. Send a plaintext message.
3. The bot should reply with a peertrans; ResonChat merges it under your bubble.
