# resonbot development notes

## Layout

| Package | Role |
| --- | --- |
| `cmd` | CLI, dial, reconnect loop |
| `internal/bot` | Multi-room session, occupancy, feature wiring |
| `internal/antispam` | Spam heuristics → warn / ban / retract-id verdicts |
| `internal/translate` | DeepL / OpenAI / Anthropic HTTP clients + prompts |
| `internal/peertrans` | peertrans stanza helpers |
| `internal/config` | TOML load / validate |
| `internal/reconnect` | Conversations-style backoff / ping timing |
| `vendor/mellium_xmpp` | XMPP standards (SM, MUC admin/role, XEP-0425 moderation, …) |

Optional features are normal packages toggled from config (`*.enabled`), not a
plugin registry.

## Robustness notes

* Ban / moderate / announce errors are logged and swallowed (bot must not die
  when it lacks moderator rights).
* First message from a user only seeds antispam state (no penalty).
* Periodic `antispam.clean_cache` wipe avoids unbounded per-user maps.
* User state is dropped after a ban.
* Translation runs off the XMPP serve goroutine; panics are recovered.
* Join failures are per-room; other rooms still join.

## Build / test

```bash
go test -mod=mod ./...
go run -mod=mod ./cmd -config config.toml
```

`go.mod` replaces `mellium.im/xmpp` with `./vendor/mellium_xmpp`.
