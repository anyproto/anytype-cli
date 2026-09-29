# Plan: support API v2 and granular API key access (anytype-heart v0.51.3)

Revision 2. It includes the findings of a three-lens review (correctness,
security, UX/rollout) against the heart `v0.51.3` tag.

## Background

anytype-heart v0.51.3 ships the JSON API v2 (`/v2/*`) and per-key access
control for API keys.

**What changed in heart (v0.50.4 → v0.51.3):**

- `model.Account.Auth.AppInfo` gained **one** field, `grant`
  (`AppGrant`). `expireAt`, `scope` and `isActive` already existed. What's
  new is that heart now validates `expireAt` when a key is created (must be
  0 or a future unix time in **seconds**) and checks it on every request.
- `AppGrant` = `spaceIds` **or** `allSpaces`, plus `perm`
  (`Read` = 0, `ReadWrite` = 1). Because `Read` is the zero value, **a grant
  without a `perm` is read-only**.
- `AccountLocalLinkCreateApp` checks that the name is set and at most 128
  bytes, that the scope is `Limited` or `JsonAPI` (never `Full`), that
  `expireAt` is valid, and that the grant has the right shape. Only
  `JsonAPI` keys can have a grant.
- New `AccountLocalLinkUpdateApp(appHash, grant)` replaces the **whole**
  grant, and sending no grant removes it. It can't change scope, expiry or
  name. It rejects a grant on a key that isn't `JsonAPI`.
  Heart evicts cached HTTP sessions, so the change applies from the next
  request.
- New `AccountLocalLinkApproveChallenge`, which only the desktop UI can
  call (`Full` scope). The **gRPC-session** pairing flow is deprecated, but
  JSON API auth still uses the same challenge flow.
- Key format: v0.51.3 creates prefixed `JsonAPI` keys and saves keys that
  carry a grant in a v2 file format. **v0.50.4 can read neither.**

**Which keys each API accepts:**

| Key | `/v1` | `/v2` | direct gRPC |
|---|---|---|---|
| `Limited` (what the CLI creates today) | ✅ | ❌ 403 "create a new api key with JsonAPI scope" | ✅ `limitedScopeMethods` allowlist (ObjectSearch, ListenSessionEvents, …) |
| `JsonAPI`, no grant | ✅ | ✅ (heart calls it "legacy", suggests reissuing) | ❌ |
| `JsonAPI`, `allSpaces` + `ReadWrite` | ✅ (tech space **included**) | ✅ (tech space **excluded**) | ❌ |
| `JsonAPI`, any other grant | ❌ | ✅ within the grant | ❌ |

On `/v1`, only keys with no grant or `allSpaces` + `ReadWrite` get through
(`ensureUngrantedKey` / `ApiGrant.IsUnrestricted`). An `allSpaces` grant
never covers the tech space on `/v2`; the tech space has to be listed
explicitly.

**Other changes in v0.51.3 that affect the CLI:**

- **Go 1.26.5** is required (`go.mod`). The CLI is on 1.25.7, and CI,
  release and CodeQL use `go-version: "1.25"`.
- The HTTP API (`/v1` and `/v2`) now checks Host and Origin. Non-localhost
  hostnames need `ANYTYPE_API_ALLOWED_HOSTS`, and browser origins need
  `ANYTYPE_API_ALLOWED_ORIGINS`. This affects the remote and reverse-proxy
  setups the README recommends.
- Heart's own gRPC-Web proxy (`cmd/grpcserver/proxy.go`) rejects requests
  from untrusted origins/hosts before they reach any handler, and turns
  WebSockets off by default (`ANYTYPE_GRPCWEB_ALLOWED_ORIGINS`,
  `ANYTYPE_GRPCWEB_ALLOWED_HOSTS`, `ANYTYPE_GRPCWEB_ENABLE_WEBSOCKETS`).
  The CLI's `core/grpcserver/server.go` accepts every origin and allows
  WebSockets.
- The tantivy version stays at v1.0.6, so no change is needed there.

## Goals

1. Keys created by the CLI work on `/v2`.
2. Users can create keys limited to chosen spaces, read-only access, and an
   expiry date. The CLI never creates a key with more access than the user
   asked for.
3. Users can see and change a key's access. Widening access requires
   explicit confirmation.
4. Existing setups keep working after upgrading, or there's a documented
   migration: `/v1` keys, gRPC integrations, remote/proxy deployments.

## Non-goals

- A CLI client for the `/v2` REST endpoints.
- Pairing approval on a headless server (see Phase 6). Headless users create
  keys directly.

## Key decisions

- **D1 — No default access: the user must choose spaces and permission
  (option C).** `apikey create` needs:
  - exactly one of `--space <id|name>` (repeatable) or `--all-spaces`, and
  - exactly one of `--read-only` or `--read-write`.

  If either choice is missing, it creates nothing and prints what's needed,
  with the user's spaces (name + ID) for `--space`. The CLI never picks which
  spaces a key can reach or whether it can write.
  - This is a **breaking change** for scripts that run
    `apikey create <name>` without flags. They fail loudly instead of
    getting a key with broad access. The release notes say so and show the
    `--all-spaces --read-write` equivalent of today's behaviour.
  - Rejected: A (no grant by default, which heart treats as legacy) and B
    (`allSpaces` by default, which gives broad access without an explicit
    decision).
- **D1a — The CLI never creates or sets a key with no grant.** Every key it
  creates, and every `update` it sends, carries a grant. There's no
  `--unrestricted` flag. The only thing a no-grant key adds over
  `--all-spaces` is `/v2` access to the tech space, and that stays possible by listing it
  explicitly with `--space <techSpaceId>`. Existing no-grant keys (made by
  other clients) are shown and can be narrowed, but never re-created or
  widened back to no grant.
- **D2 — Never trust the server to apply what was asked.** After any create
  or update, read the key back with `ListApps` and compare. If the result
  doesn't match, revoke (for create) and fail. Together with a server
  version check beforehand, this closes the gap where a new CLI talks to an
  older running service (see Phase 2).
- **D3 — No secrets in listings.** `list`, including `--json`, prints
  selected fields only. It never prints `appKey`. Only `create` prints the
  secret, once.
- **D4 — `update` merges with the current grant.** The CLI reads the
  current grant, applies only the dimensions the user changed (spaces or
  permission), and sends the whole result. It never sends an empty grant
  (D1a).

---

## Phase 1: dependency, toolchain and transport upgrade

**Files:** `go.mod`, `go.sum`, `.github/workflows/{ci,release,codeql}.yml`,
`Makefile` / Dockerfile if they pin Go, `core/grpcserver/server.go`, `README.md`,
`SELF-HOSTED.md`, `CLAUDE.md` (Go version)

- [ ] `go get github.com/anyproto/anytype-heart@v0.51.3 && go mod tidy`
      (this raises the `go` line to `1.26.5`).
- [ ] Move CI, release and CodeQL to Go 1.26.5+. Check that the
      `golangci-lint` version supports it. Update build requirements in
      `CLAUDE.md` / `README.md` and the Dockerfile base image.
- [ ] Copy heart's `replace` directives (Go ignores them in dependencies):
  - [ ] `github.com/libp2p/zeroconf/v2 => github.com/anyproto/zeroconf/v2 v2.2.1-0.20260709212715-528971bb5854` (changed in v0.51.x)
  - [ ] Add the ones the CLI was already missing, and check each one:
        `gogo/protobuf`, `dgraph-io/badger/v4`, `dgraph-io/ristretto`,
        `multiformats/go-multiaddr`, `genproto/googleapis/rpc`,
        `araddon/dateparse`, `dsoprea/go-jpeg-image-structure/v2`
        (exact versions: heart `v0.51.3:go.mod`).
- [ ] **gRPC-Web transport:** bring over heart's `proxy.go` policy. Build
      the `localorigin` policy from `ANYTYPE_GRPCWEB_ALLOWED_{ORIGINS,HOSTS}`
      (allowing file:// and the webclipper extension, as heart does). Reject
      untrusted requests before they reach any handler. Turn WebSockets off
      unless `ANYTYPE_GRPCWEB_ENABLE_WEBSOCKETS=1`. Also add heart's
      interceptor that copies the Origin header from gRPC metadata into the
      request context.
- [ ] **Remote/proxy setups:** document `ANYTYPE_API_ALLOWED_HOSTS` /
      `ANYTYPE_API_ALLOWED_ORIGINS` (and the gRPC-Web equivalents) in
      `README.md` and `SELF-HOSTED.md`, including how to set them for
      `anytype service install`.
- [ ] Verify with `make build`, `make test`, `make lint`, and cross-compile
      for every release platform.
- [ ] Smoke tests:
  - `/v1` and `/v2` respond on `localhost:31012`.
  - Through a custom hostname or reverse proxy: rejected by default,
    accepted once the allow-list is set.
  - gRPC-Web: a disallowed Origin gets 403; a WebSocket upgrade gets 403 by
    default.

**Estimate:** 1–1.5 days (toolchain + transport + deployment docs; the
dependency bump itself is the small part).

---

## Phase 2: switch keys to `JsonAPI`, safely

**Files:** `core/apikey.go`, `cmd/auth/apikey/create/create.go`

- [ ] Create keys with `Scope: model.AccountAuth_JsonAPI` and a grant.
- [ ] **Space and permission choices are required (D1):**
  - Add `--space <id|name>` (repeatable) / `--all-spaces`, and
    `--read-only` / `--read-write`. Exactly one from each pair must be
    given.
  - If either is missing, fail with usage (listing the user's spaces when
    the space choice is missing). Nothing is sent to the server.
  - Resolve spaces as described in Phase 3 → Rules.
  - `Perm` is always set explicitly from the flag.
  - These flags come in this PR, not Phase 3, because switching to `JsonAPI`
    needs a grant and the CLI won't pick one for the user.
- [ ] **Server capability check before creating a key:** call
      `AppGetVersion`. If the running server is older than v0.51.3, or
      reports no version, stop with *"the running Anytype service is older
      than this CLI; run `anytype service restart`"*. `anytype update` swaps
      the binary without restarting the service, so this situation is
      common. v0.50.4 would silently drop the grant and the expiry.
      *Check first:* does the embedded heart report its real version through
      `AppGetVersion` (ldflags)? If not, compare the CLI's own version,
      exposed by the server, instead.
- [ ] **Read back after creating (D2):** `ListApps` → find the new key by
      name + creation time (or by `appHash` if CreateApp returns it) →
      compare scope, grant and expiry. On any difference: `RevokeApp` and
      return an error. This protects against any server that ignores fields.
- [ ] Manual check: the new key gets 200 from `GET /v2/auth/whoami` and from
      `GET /v1/spaces`.

**Migration guidance (ships in this PR, README "Upgrading to API v2"):**

1. `JsonAPI` keys **cannot call gRPC methods directly**. Integrations that
   use a `Limited` key over gRPC (ObjectSearch, ListenSessionEvents, …)
   should keep that key.
2. Moving a REST integration to `/v2`: create the new key → check the client
   works → switch its credentials → `apikey revoke` the old one.
3. Keep **the same key name** when replacing a key. On `/v2`, deleting an
   object requires the key's name to match the name recorded when the
   object was created, so renaming loses the ability to delete what the old
   key created.
4. Downgrading: keys created or updated by v0.51.3+ **don't work** on
   v0.50.x. Keep the old keys until the upgrade is confirmed.

**Estimate:** 1 day (includes resolving spaces).

---

## Phase 3: expiry and output flags on `apikey create`

**Files:** `cmd/auth/apikey/create/create.go`, `core/apikey.go`, new
`core/apikeygrant.go` (parsing flags, resolving spaces, describing
grants and compatibility)

### Command

```
anytype auth apikey create <name>
    (--space <id|name>... | --all-spaces)   # required, exactly one (Phase 2)
    (--read-only | --read-write)            # required, exactly one (Phase 2)
    [--expires <dur|date>]     # e.g. 30d, 12h, 2026-12-31
    [--json]                   # {"id","name","key","scope","grant","expiresAt","compat":{"v1","v2"}}
```

### Rules

- Exactly one of `--space` / `--all-spaces` and exactly one of
  `--read-only` / `--read-write` are required; the flags in each pair can't
  be combined (D1). A space picker in the terminal could come later; a missing
  choice is never filled in automatically.
- There's no way to create a key without a grant (D1a).
- **Resolving spaces:**
  - Resolve every argument once, before anything is sent, to a **full
    space ID** using `core.ListSpaces`.
  - An exact full ID takes priority over a name. Names must match exactly
    one space; if a name matches several or none, fail and list the
    candidates (name + ID).
  - v2 short references are rejected.
  - Duplicates are removed. The tech space is only reachable by its explicit
    ID, with a warning.
  - The resolved list (name + full ID) is what's displayed and what's sent.
- **Expiry:** Go durations + a `d` suffix, or `YYYY-MM-DD` (end of day,
  local time), converted to unix **seconds**. Must be in the future. Heart
  treats a key as expired once `now > expireAt`.
- **Name:** not empty, ≤ 128 bytes, never truncated. Warn if another key
  already has the same name: on `/v2` both keys would be able to delete each
  other's objects within their grants.

### Output

- The secret is printed **once**, on its own line. With `--json`, it goes
  in the `key` field.
- Print the scope, a readable grant (spaces by name + short ID, read or
  read-write), the expiry, and **which APIs the key works with** (`v1 ✅/❌`,
  `v2 ✅`) using the table in Background. For example,
  `--all-spaces --read-only` → `v1 ❌`.
- The docs show how to pipe or capture the key. Examples never put a real
  key on the command line.

**Estimate:** ~1 day including tests.

---

## Phase 4: `apikey list` and `apikey update`

### 4a. `apikey list`

**Files:** `cmd/auth/apikey/list/list.go`

- [ ] Columns: `NAME  ID  SCOPE  ACCESS  EXPIRES  APIS  SESSION  CREATED`
  - `ACCESS`: `all:rw`, `all:ro`, `<n> spaces:rw|ro`, `—` for `Limited`,
    or `unrestricted` for existing no-grant `JsonAPI` keys created by other
    clients (never shown as "full", which could be confused with `Full`
    scope).
  - `EXPIRES`: `never`, a date, or `expired`.
  - `APIS`: which APIs the key works with, e.g. `v1,v2`, `v2`,
    `v1,grpc` (`Limited`).
  - `SESSION`: `active` if heart has a live session for the key
    (`isActive`). This doesn't mean the key is valid.
- [ ] Drop the key-prefix column.
- [ ] `--json`: selected fields only (D3), and **no `appKey`**. Emit `[]`
      when there are no keys, RFC 3339 timestamps, and the full grant with
      space IDs. Diagnostics go to stderr.
- [ ] A footer hint for `Limited` keys: they don't work on `/v2`; link to the
      migration section.
- [ ] A footer hint for no-grant `JsonAPI` keys: suggest narrowing them with
      `apikey update <id> --all-spaces` (or `--space …`).

### 4b. `apikey update <id|name>` (new)

**Files:** new `cmd/auth/apikey/update/update.go`, registered in
`cmd/auth/apikey/apikey.go`; `core/apikey.go` gets `UpdateAPIKeyGrant`

```
anytype auth apikey update <id|name>
    [--space <id|name>]... | [--all-spaces]          # replace the spaces
    [--add-space <id|name>]... [--remove-space <id|name>]...   # edit the list
    [--read-only | --read-write]
    [--yes]
```

This is a separate command, not a flag on `create`: `create` makes a new
secret and requires every choice, while `update` keeps the same key and
changes only what you pass.

**Finding the key:** the argument is matched first as an exact key ID
(`appHash`), then as an exact key name. If several keys have that name,
fail and list them (name, ID, created) so the user can pass the ID.

**Space flags:**

- `--space` / `--all-spaces` replace the whole set and can't be combined
  with `--add-space` / `--remove-space`.
- `--add-space` / `--remove-space` edit the current list and can be used
  together. Both resolve names like `--space` (Phase 3 → Rules).
  - Adding a space the key already has, or removing one it doesn't have,
    is a no-op for that space; the command says so.
  - Removing the **last** space → error pointing to
    `apikey revoke <id>`, because heart rejects an empty grant.
  - On an `allSpaces` key (or an existing no-grant key), there's no list to
    edit. `--remove-space` fails and suggests `--space <the spaces to
    keep>…`. `--add-space` of an ordinary space is a no-op (already
    included); `--add-space <techSpaceId>` fails and suggests `--space` with
    an explicit list, since `allSpaces` never covers the tech space.

How it works (D4):

1. Find the key (above) via `ListApps`. If it isn't `JsonAPI`, refuse and
   point to recreating it (heart rejects grants on `Limited` keys).
2. Work out the new grant from the current one:
   - `--space` **replaces** the set of spaces; `--all-spaces` sets it to
     all spaces; `--add-space` / `--remove-space` edit the current list.
     With none of these, the current spaces are kept.
   - `--read-only` / `--read-write` set the permission. With neither, the
     current permission is kept.
   - A key with no grant counts as "all spaces including the tech space,
     read-write". So a permission flag alone → `allSpaces` + that
     permission; `--space` alone → those spaces + `ReadWrite`. Every result
     is a narrowing.
   - The result always has a grant (D1a); no flag can remove it.
   - No flags that change anything → error. If the new grant equals the
     current one → "no change", exit 0.
3. Show **before → after** (spaces by name + ID, permission, which APIs the
   key works with, including losing `/v1`).
4. If the change widens access (below), ask for confirmation. Without a
   TTY, refuse unless `--yes` is given.
5. Just before sending, **read `ListApps` again**. If the grant changed
   since step 1, stop and ask the user to retry. Heart has no conditional
   update, so this only narrows the race window; it doesn't close it (see
   the upstream ask below).
6. Send the whole grant, then read it back (D2).

**When a change widens access.** It widens access if the new grant allows
any capability the old one didn't. Compare with space IDs deduplicated, and
treat `allSpaces` as "all current and future spaces except the tech space":

- any space in the new set that isn't in the old one (`{A}→{B}` widens even
  though the count is the same);
- a list of spaces → `allSpaces` (adds future spaces, even if the list had
  every current space);
- `allSpaces` → a list containing the tech space;
- `Read` → `ReadWrite` on any space still granted (even if other spaces
  are removed);
- anything that makes the key usable on `/v1` again.

**Documented limitations:**

- `update` can't change the scope, the expiry or the name; recreate the key
  for those.
- A key can't be widened back to no grant. For `/v2` tech-space access,
  list the tech space with `--space`.
- An open `/v2` chat stream keeps its old grant and isn't cut off at expiry
  until it reconnects (heart only checks at the start of a request).

**Upstream asks (heart), to file separately:**

- `UpdateApp` with an "expected current grant" (compare-and-swap);
- cancel open streams when a key expires or its grant is edited;
- confirm that `AppGetVersion` reports the embedded heart version.

**Estimate:** 1.5–2 days including tests.

---

## Phase 5: shell mode (required)

**Files:** `cmd/shell/shell.go`

The new flags make existing shell-mode bugs matter:

- [ ] Parse input with quotes (e.g. `github.com/google/shlex`), so
      `--space "My Team"` works.
- [ ] Reset flag values between commands: create the command tree for every
      line, or reset each flag and its `Changed` state. Otherwise `--yes` or
      `--space` from one command carries over into the next.
- [ ] Confirmation prompts work in the shell (they read the same TTY).
- [ ] Make completion include the `apikey` subcommands and their flags
      (it currently only goes two levels deep).

**Estimate:** 0.5 day. Ships with PR 2/3, not as a follow-up.

---

## Phase 6: optional follow-ups

- [ ] **Pairing approval on a headless server.** The pairing challenge is
      still how the **JSON API** authenticates, and on a headless server
      nothing can approve it. Recommendation: **defer** until a concrete
      integration can't accept a key created manually. If it's built, it
      must:
  - show the caller exactly as the event reports it (process path, origin)
    and the permission it asked for;
  - collect an explicit scope and grant, resolved as in Phase 3;
  - require explicit approval;
  - show the returned 4-digit code **only** to the person approving,
    never solving it automatically or writing it to logs.

  Keys paired this way can't have an expiry.

---

## Testing

The repo's tests use the standard `testing` package, so follow that style.
Table-driven cases:

- **Parsing `--expires`:** durations, `d` suffix, dates, past/now/negative
  values, garbage.
- **Building a grant from flags:** every valid and invalid combination,
  that `Perm` is always set explicitly, and that a missing space choice or
  a missing permission choice is an error that sends nothing to the
  server.
- **Never sending an empty grant (D1a):** every `create` and `update` path
  sends a grant that isn't nil, including `update` on an existing no-grant
  key.
- **Resolving spaces:** exact ID, name, ambiguous, unknown, duplicates, the
  tech space, short references.
- **Merging for `update`:** keeping the current value when a flag is
  omitted, replacing the set, permission only on a key with no grant, no
  change, conflicting flags.
- **`--add-space` / `--remove-space`:** adding to and removing from a list,
  both in one command, a no-op add or remove, removing the last space
  (error), `--remove-space` on an `allSpaces` or no-grant key (error), adding
  the tech space to an `allSpaces` key (error), combining with `--space` or
  `--all-spaces` (error).
- **Finding the key for `update`:** exact ID, exact name, a name shared by
  several keys (error listing them), unknown.
- **Detecting widened access** — every case in Phase 4b, plus the matching
  narrowing cases, including `all:ro → [tech]:ro` and `[A,B]:ro → A:rw`.
- **Which APIs a key works with:** no grant, all/rw, all/ro, a list of
  spaces, `Limited`, expired.
- **Output:** neither the table nor `--json` from `list` contains any key.
  `create --json` contains it exactly once.
- **Read-back (D2):** a fake server that drops the grant or expiry → the key
  is revoked and the command fails.
- **Shell:** consecutive commands don't carry flag values over; quoted
  arguments work.

Manual end-to-end against `anytype serve`:

1. `--space A --read-only`: `/v2` read in A → 200; write in A → 403; space
   B → 403; `/v1` → refused.
2. `--all-spaces --read-only`: `/v2` read → 200; `/v1` → refused.
3. `--all-spaces --read-write`: `/v1` + `/v2` → 200; `/v2` tech space →
   403. No space flag, or no permission flag → error, no key created (with
   and without a TTY).
4. After a first request (so heart caches the session), `update` narrows
   RW→RO and A+B→A → the next request is denied, including losing `/v1`.
5. `update` that widens access → asks for confirmation; cancelling changes
   nothing; without a TTY and without `--yes` → refused, nothing changes.
6. `--expires 1m` → 401 "expired" after a minute, including for a key
   whose session is already cached. The key and its grant survive a service
   restart.
7. An old `Limited` key: `/v1` 200, `/v2` 403, gRPC methods on the allowlist
   still work; `update` on it → refused, pointing to recreating it.
8. A new CLI talking to a **v0.50.4** service that is still running →
   `create --space X` fails and **no key is created**.
9. Downgrade: an existing old key still works on v0.50.4; the release notes
   say keys created by the new CLI don't.

## Rollout

1. **PR 1:** Phases 1 + 2. The toolchain, dependency bump, transport
   protection, `JsonAPI` scope with required space and permission choices,
   the version check,
   read-back after create, and the migration and deployment docs.
2. **PR 2:** Phase 3 + 4a + the shell parsing/flag-reset parts of Phase 5.
3. **PR 3:** Phase 4b + the rest of Phase 5.
4. The upstream heart asks and Phase 6 as separate issues.

Release notes for PR 1:

- the Go toolchain requirement;
- Host/Origin allow-lists for remote setups;
- gRPC-Web WebSockets are now off by default;
- new keys are `JsonAPI` (no gRPC access);
- **breaking:** `apikey create` needs `--space …` or `--all-spaces`, and
  `--read-only` or `--read-write`;
- the downgrade caveat;
- `anytype service restart` after updating.

**Total estimate:** ~6–7 days.

## Open questions

1. Should `apikey create` also offer `--scope limited`, for integrations that
   still need gRPC access? The recommendation is no unless someone asks for
   it; point them to existing keys.
2. Does the embedded server report heart's version through
   `AppGetVersion`? This determines how the Phase 2 version check is built.
3. Headless pairing approval (Phase 6): is it needed by Raycast, MCP or
   web-clipper users on servers?
