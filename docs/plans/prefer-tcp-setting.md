# Spec: `prefer-tcp` setting and a user-facing `anytype config`

## Background

Heart dials sync nodes and peers over QUIC (UDP) first and falls back to
TCP (yamux) within the same dial (`any-sync` `peerservice.Dial` tries every
address in order). On networks that block or throttle UDP, every new
connection first waits for a QUIC attempt to fail. Heart also demotes single
peers to TCP-first when their QUIC connections keep dying
(`ANYTYPE_QUIC_AUTO_DEMOTION`, GO-7467), but that only kicks in after repeated
failures.

`AccountCreate` and `AccountSelect` accept `preferYamuxTransport`. When set,
heart skips `PreferQuic(true)` and dials TCP first, keeping QUIC as the
fallback. Heart reads it once, when the account starts.

The CLI never sets the field. The only way to use it today is the misspelled
heart variable `ANYTYPE_PEFERYAMUXTRANSPORT=true`, read by heart's
`envconfig`. It only reaches processes started from that shell, and nothing
in the CLI reports it.

The CLI config (`~/.anytype/config.json`) mixes account state (account Id,
tech space Id, network Id), fallback credentials (account key, session token)
and settings (JSON API listen address). `ClearAccount` keeps settings by
copying `ApiListenAddr` by hand, so any new setting would be wiped on logout.

A hidden `anytype config get|set|reset` exists. It only knows `accountId` and
`techSpaceId`, and `reset` deletes fallback credentials and the saved listen
address.

## Goals

1. Users can turn on TCP-first dialing with one documented command, and it
   persists across restarts and logout.
2. Docker and one-off tests can turn it on with an environment variable.
3. `anytype auth status` and `anytype inspect --json` show the effective value
   and where it comes from.
4. `anytype config` becomes a public command for user settings only.

## Non-goals

- Applying the change to a running account. It applies at the next account
  start; the CLI tells the user how to restart.
- Reporting the transport the running account actually uses. Heart doesn't
  expose it; the CLI reports the configured value.
- A `--prefer-tcp` flag on `serve`, `service install`, `auth login` or
  `auth create`.
- A machine (`--json`) interface for `anytype config`.

## Design

### Name

User-facing name is `prefer-tcp` (config key), `preferTcp` (JSON field) and
`ANYTYPE_PREFER_TCP` (environment). "TCP" is what users recognize; yamux is an
implementation detail.

### Storage

`config.Config` embeds a new `Settings` struct:

```go
type Settings struct {
	ApiListenAddr string `json:"apiListenAddr,omitempty"`
	PreferTcp     bool   `json:"preferTcp,omitempty"`
}
```

Go flattens embedded structs in JSON, so the file format doesn't change and
existing files load unchanged. `ClearAccount` keeps the whole `Settings`
value and deletes the file only when `Settings` is empty, so future settings
survive logout without further changes.

`false` and "not set" are the same; `false` is not written.

### Resolution

```
ANYTYPE_PREFER_TCP (non-empty)  >  preferTcp in config.json  >  default (false)
```

- The environment value is parsed with `strconv.ParseBool`. An empty value
  counts as unset.
- An invalid environment value is ignored with a warning and resolution
  continues with the saved value. Login must not fail on it, because the
  service's auto-login uses the same code.
- `config.ResolvePreferTcp(Settings)` returns `(value, source, err)` where
  source is `env`, `config` or `default`. `config.PreferTcp()` loads the saved
  config first.

The environment variable affects only the process that sends the login
request: a foreground `serve`, a Docker container, or a login command run
from that shell. The background service doesn't inherit shell variables, so
the persistent choice is `config set`.

### Where it's sent

`PreferYamuxTransport` is set on every account start the CLI makes:

- `core.Authenticate`: `AccountSelect` (used by `auth login` and the service's
  auto-login on start).
- `core.CreateWallet`: `AccountCreate` and `AccountSelect` (`auth create`).
- `core/automation/account.go`: `AccountCreate` and `AccountSelect`. An
  invalid environment value fails the operation with
  `invalid_environment` before any mutation, since automation fails closed.

The value is not part of the automation operation's settings digest:
changing it between retries doesn't affect account identity or bindings.

Because the service's auto-login goes through `core.Authenticate`,
`anytype service restart` applies a changed setting. The service definition
doesn't change.

### `anytype config`

The command is unhidden and manages user settings only:

```
anytype config list                 # every key, its effective value and source
anytype config get <key>            # the effective value only, for scripts
anytype config set <key> <value>
anytype config unset <key>          # back to the default
```

Keys:

| Key | Values | Env override | `set`/`unset` |
|---|---|---|---|
| `listen-address` | `host:port` | — | refused, with a hint to use `--listen-address` on `service install`, `serve`, `auth login` or `auth create` |
| `prefer-tcp` | `true`/`false` (anything `strconv.ParseBool` accepts) | `ANYTYPE_PREFER_TCP` | allowed |

`listen-address` is read-only here because the installed service definition
can carry its own `--listen-address`; changing only the config would leave the
service and the account disagreeing on the JSON API address (and automation
reports `service_binding_conflict`).

Output:

```
$ anytype config list
KEY             VALUE            SOURCE
listen-address  127.0.0.1:31012  default
prefer-tcp      true             config

$ anytype config set prefer-tcp true
✓ prefer-tcp = true
Applies the next time the account starts. To apply it now, restart the server:
  anytype service restart   (or stop and start 'anytype serve')
```

When `ANYTYPE_PREFER_TCP` is set in the current environment, `set` and
`unset` warn that it overrides the saved value for commands run here.

Removed:

- `config get|set accountId|techSpaceId`: internal state, shown by
  `auth status` and `inspect --json`.
- `config reset`: it deleted fallback credentials and the listen address.
  `config unset <key>` replaces it.

`config list` joins `config get` as a read-only command for the instance lock
(`localstate.HumanMutation`).

### Status

`anytype auth status` prints `  - Prefer TCP: <value> (<source>)` when the
server is running, and the warning when the environment value is invalid.

`inspect --json` (and `auth status --json`, `service status --json`, which
share it) gains:

```json
"transport": {"prefer_tcp": false, "source": "default", "observation": "configured"}
```

`source` reflects the calling process's environment. An invalid
environment value adds `next_action`.

### Docs

- README: a Configuration section (`config` commands, keys, env override)
  and a troubleshooting line for stalled sync on networks that block UDP.
- SELF-HOSTED.md: a troubleshooting entry for nodes whose QUIC (UDP) ports
  aren't reachable.
- AUTOMATION.md: document `transport` in the inspection result and
  `invalid_environment`.
- CLAUDE.md: list the `config/` command group.

## Tests

- `config`: `Settings` JSON round-trip keeps the flat format;
  `ClearAccount` keeps every setting and deletes the file when none remain;
  `UpdateSettings` persists.
- `ResolvePreferTcp`: table over env unset/empty/true/false/invalid ×
  stored true/false.
- Setting registry: `set`/`unset` for `prefer-tcp`, invalid value, read-only
  `listen-address`, unknown key.
- `cmd/config`: `list`, `get`, `set`, `unset` against a temp `HOME`.
- `localstate.HumanMutation`: `config list` and `config get` don't lock;
  `config set` and `config unset` do.
- Automation: `AccountSelect`/`AccountCreate` requests carry
  `PreferYamuxTransport` from config and env; invalid env fails with
  `invalid_environment`; `inspect` reports `transport`.
