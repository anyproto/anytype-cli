# Native CLI automation v1

The additive `--json` interface uses the existing CLI command paths. It never
prompts, prints colors/update hints, or interprets private input as shell text.
Human commands without `--json` retain their existing presentation and flags.
Machine help and parse errors are also single JSON objects on stdout.

This interface manages **one default native instance**. A Hermes profile is not
an Anytype instance. `--instance default` is accepted; all other selections fail
with `unsupported_instance`. Config/data bindings are derived from the ordinary
CLI configuration and reported explicitly. Service automation supports launchd
user agents and systemd user services; Windows service automation is unsupported
in this first version. Arbitrary service environments, systemd drop-ins, custom
service definitions, and `DATA_PATH` service provisioning are not adopted.
Ordinary Windows CLI/service operation remains supported; the machine service
adapter limitation does not disable its runtime observation or credential store.
Windows JSON account create/login and inspection can use a nonce-verified
foreground `serve`; inspection reports `service.manager:unsupported` without
claiming a verified service definition. Machine service install/start still
return `unsupported_service_platform`.

## Envelope and exits

Every machine command emits exactly one object:

```json
{"schema_version":1,"state":"ready","changed":false,"result":{},"next_action":""}
```

Errors add `error: {code, message, retryable}`. Error messages are controlled CLI
text and typed protobuf code names; raw server descriptions, command arguments,
private inputs, and secret fragments are never copied into diagnostics. Result
fields use snake_case. Unknown fields should be ignored; unknown schema versions
must be rejected. `changed` is true when this invocation positively observed a
mutation; false is **not** a no-side-effect guarantee after an interrupted RPC.
Read the error and operation stage when a result is uncertain.

| Exit | Meaning |
| --- | --- |
| 0 | Requested operation/inspection completed |
| 2 | Input or operator action required |
| 3 | Membership request sent, pending approval, or syncing |
| 4 | Temporary unavailability/deadline/instance contention |
| 1 | Incompatible or terminal outcome |

States include `ready`, `needs_input`, `unavailable`, `incompatible`,
`request_sent`, `pending_approval`, and `syncing`. Typed errors include
`invalid_arguments`, `operation_id_required`, `operation_conflict`,
`operation_uncertain`, `instance_busy`, `unsupported_instance`,
`instance_mismatch`, `service_upgrade_required`, `account_mismatch`,
`account_transition`, `authentication_required`, `grant_mismatch`, and
`unverified_key_cleanup_uncertain`. Heart errors become `rpc_<lowercase code>`.
An invitation-not-found code is not labeled expired: Heart does not distinguish
expiration, revocation, missing content, and every network failure reliably.

All commands accept `--json`, `--non-interactive` (redundant),
`--instance default`, `--expect-account ID`, and `--timeout 45s` (1s–5m).
The caller's whole-command deadline is respected. Individual liveness and OS
status probes are bounded more tightly. CLI keyring operations use the platform
credential API. A stalled platform keyring can require caller process timeout.

## Read-only discovery

```text
anytype capabilities --json
anytype inspect --json
anytype service status --json
anytype auth status --json [--expect-account ID]
anytype space list --json [--expect-account ID]
anytype space status SPACE_ID --json [--expect-account ID]
anytype auth apikey list --json [--expect-account ID]
```

`capabilities` describes the **embedded build**, with no network/config/keyring
access. `inspect`, `service status`, and `auth status` share an inspection result:

- `cli_version`, `capabilities`;
- `instance`: `id`, absolute `config_path`, `data_path`, `work_path`, fixed
  `grpc_address`, `grpc_web_address`, and `isolated:false`;
- `service`: `name`, `manager`, `definition_path`, `definition_hash`,
  `executable`, `installed`, `running`, `binding_verified`, `listen_address`,
  supervisor `pid` and separately nonce-verified `runtime_pid` when available, and `health`;
- `account`: full `id`, observed `display_name`, `authenticated`,
  `observation` (`live` or `configured`), and credential-storage backend;
- `network`: `id`, `config_path`, and `observation`;
- `transport`: `prefer_tcp`, its `source` (`env`, `config` or `default`), and
  `observation:configured`. It is the value the next account start from the
  calling process would send; `env` is the caller's `ANYTYPE_PREFER_TCP`. Heart
  does not report the transport the running account uses. An invalid
  `ANYTYPE_PREFER_TCP` adds `next_action`;
- `api`: effective `origin`, `listen_address`, `bound`, `observation`, and
  `ready:false`/`readiness:requires_integration_key_probe`, plus
  `binding_observation` and an actionable `next_action` when unverified;
- `mcp`: effective `url`, bearer authentication, `token_source:same_api_key`,
  `same_grant:true`, `ready:false`/`readiness:not_probed`.

Configured facts remain available in `result` when the service is unavailable.
A successful supervisor start is independent of API binding: Heart starts the
HTTP API on account login. HTTP/MCP usability must be probed with the selected
integration credential. CLI inspection does not mint a temporary API key.

The existing embedded service now publishes a sanitized private atomic
`~/.anytype/automation/runtime.json`. Its random per-launch nonce must match the
live `AppGetVersion` response metadata `anytype-cli-runtime`. A PID/file alone
is never evidence of liveness. Config/data/nonce mismatches fail closed; account
transitions invalidate authentication/readiness claims. Account name is read
from the authenticated native profile index, not guessed from config.
During wallet recovery, the previous API binding is held only in process memory.
It is restored only after successful selection of the same account and data
root, with no intervening stop, rebind, accepted API status event, account
creation, different/unknown-account selection, or uncertain account start.
Selecting another account can close the old application before returning an
error, so the old binding is discarded before that selection enters Heart.
A failed or incomplete create/select response discards continuity and closes
its startup-event window, reporting `account_start_outcome_unknown`; inspection
directs the caller to resume the account operation or explicitly restart the
native service. A new account never inherits the previous account's binding.
Native `AccountMove` and `AccountChangeNetworkConfigAndRestart` also invalidate
binding continuity before entering Heart and participate in overlap tracking.
A successful move observes a stopped account. Network restart has no account or
API status in its response, so it leaves `account_start_outcome_unknown` even on
success; a later account selection establishes identity, and an explicit API
rebind (or native service restart) establishes binding again.
Delayed service auto-login skips any account activity already observed under the
shared instance lock, including an unresolved transition.

Heart broadcasts API status asynchronously without an event generation or
account identity. Non-overlapping `AccountChangeJsonApiAddr` outcomes supersede
broadcasts, including a successful disable and a failed bind. An interrupted or
failed rebind without a status leaves the binding unverified, because shutdown
may already have happened. Stop/shutdown discard retained binding evidence when
they begin, even if closing components later reports an error; a failed stop
reports `stop_outcome_unknown` and cannot restore the old binding on selection.
The observer never queues native lifecycle handlers behind one another: newer
starts, stop, and shutdown reach Heart immediately to supersede/cancel recovery.
Overlapping lifecycle calls have no proven native effect order, so their batch
reports `concurrent_lifecycle` and suppresses positive account/API response and
event claims. A later independent account observation or explicit service restart
can resolve an account transition; an independent rebind can resolve uncertainty
from overlapping API-only rebinds. Inspection explains the applicable next action.
Delayed success/error events cannot replace established RPC outcomes.
Normal first startup accepts a matching event for its requested fixed numeric
address, treating `0.0.0.0:PORT` and `[::]:PORT` as aliases for Go's dual-stack
wildcard listener in both matching and address history. Distinct unicast
endpoints retain separate history. Same-account recovery retains prior binding
evidence. A distinct-address
startup rejects old-address events. Reusing an address after a stop in the same
process cannot distinguish a new event from an old one: inspection reports
`binding_observation:unverified_event_order`, `bound:false`, empty API/MCP origins,
and a next action to explicitly restart the native service or rebind through
`AccountChangeJsonApiAddr`. A fresh service process and an explicit successful
rebind can establish binding evidence again. Dynamic/hostname startup addresses
also remain unverified when their event cannot be tied to a unique requested
address. None of these observations prove HTTP/MCP usability without the
integration-key probes.

**Previously running CLI binaries need an explicit upgrade/restart** before this
observation interface is available. Inspection returns `service_upgrade_required`
and never restarts/replaces them itself. There is no new daemon, listener, or
Hermes-specific RPC service.

HTTP MCP in this embedded Heart uses `/mcp/full` (also `/mcp/small` and
`/mcp/large`) on the same actual API origin. It uses the same bearer API key and
space/read-write grant as REST v2, is stateless, and supports protocol versions
`2025-06-18` and `2025-11-25`. The full tool set excludes `auth_whoami`; REST
`GET /v2/auth/whoami` remains available. These build facts are not live readiness
proof for an arbitrary externally managed service.

## Explicit service provisioning

```text
anytype service install --json --operation-id SERVICE_OPERATION [--listen-address IP:PORT]
anytype service start --json --operation-id START_OPERATION [--listen-address IP:PORT]
```

A matching existing definition is reused. Definitions with different executable,
listener, environment, or unknown binding are not replaced. Existing account
network/ports are preserved. A reachable incompatible/mismatched native server
blocks installation. A definition without `--listen-address` inherits the saved
CLI address, as `serve` does. A launchd `Program` override is unverified and
blocks automatic adoption. Systemd must report a loaded unit, no required daemon
reload, and an effective `ExecStart` matching the supported on-disk definition.
Stale, missing, or unparseable supervisor evidence blocks adoption; the CLI does
not reload an externally changed unit. Result is `{operation_id, service, api_ready:false,
next_probe}`. Start may be retried against the same supervisor definition. An
ambiguous installation with no observable definition requires intervention.
The CLI invokes its existing service ownership commands with bounded subprocesses;
it does not write a separate Hermes service.

## Accounts

```text
anytype auth create 'Агент 🌿' --json --expect-account none --operation-id CREATE_OPERATION [--include-secret] [--network-config FILE] [--listen-address IP:PORT]
anytype auth login --json --expect-account none --operation-id LOGIN_OPERATION --key-stdin [--network-config FILE] [--listen-address IP:PORT]
anytype auth login --json --expect-account FULL_ACCOUNT_ID --operation-id LOGIN_OPERATION --key-stdin
```

Stdin is a pipe or owner-only regular file containing only the account key, optionally followed by a newline, then EOF.
No raw key argument/flag is accepted. `--expect-account none` proves the instance
has no live/configured account, saved key, or existing data before first creation
or first import. Login derives the full identity locally and binds it durably to
the operation; subsequent login against an existing instance requires its full
expected identity. Credentials/account/network are never implicitly replaced.
Saved account credentials, including legacy config keys, must also derive to the
expected identity before any wallet/session/account mutation. Unreadable
credential evidence blocks provisioning. The chosen YAML/default network must
match observed and configured network identity. Same-account reuse preserves
the live API listener, including when CLI config was not yet saved.

Creation trims outer whitespace, rejects empty/invalid UTF-8/control characters,
and preserves internal whitespace and Unicode. The verified Heart account-create
and profile-details paths impose no name-specific maximum; the CLI advertises its
own 4096 UTF-8 byte input bound. Account rename is deliberately unsupported:
`auth rename NAME --json` returns `unsupported_operation` without mutation.

Result is `{operation_id, account}`. With explicit `--include-secret`, create or
login/recovery may also return `account_key`. Secret output requires a pipe or owner-only regular file,
not a terminal. Capture it only in the private calling process; do not log it.
Omitting the flag never prints any key fragment. Resume repeats the same command
and operation ID; private stdin can be omitted once the credential is stored.
Network file paths are normalized before comparing effective settings; an exact
replay of a relative path remains valid after its absolute form was saved.
The selected file's content hash is also bound to the operation.

Account creation and selection send the `prefer-tcp` setting (see
`anytype config`), overridden by `ANYTYPE_PREFER_TCP` in the calling process.
It is not bound to the operation, so a resumed operation sends the current
value. An invalid `ANYTYPE_PREFER_TCP` fails with `invalid_environment` before
any mutation.

Wallet recovery material is durably saved before account creation. Exact live
identity plus observed requested name can reconcile a lost create response.
After a confirmed account response, deterministic `AccountSelect` can recover a
restarted service without repeating `AccountCreate`. A sent-but-unconfirmed create
without such proof returns `operation_uncertain`. Partial protobuf errors retain
the recovery credential and the uncertain operation; they are not successes.

## Native space membership

```text
anytype space join --json --expect-account FULL_ACCOUNT_ID --operation-id JOIN_OPERATION --invite-stdin [--space-id EXPECTED_FULL_SPACE_ID]
anytype space join --json --expect-account FULL_ACCOUNT_ID --operation-id JOIN_OPERATION
anytype space status FULL_SPACE_ID --json --expect-account FULL_ACCOUNT_ID
```

First-call stdin is one invitation URL followed by EOF. The journal stores the
resolved space ID, never the invitation or its key. Resume can inspect the
recorded membership without stdin and without resubmission. Status/list read the
native membership index using the native session before any integration API key
exists. Full IDs are returned with name, raw local/account/participant status,
normalized `state`, and `http_ready:false`.

States include `request_sent`, `pending_approval`, `joined`, `syncing`, `rejected`,
`removed`, and `unknown`. Missing participant status is not interpreted as joining.
Heart's terminal deleted account status overrides a stale joining participant
after rejection. Loaded owned/personal spaces with local `Ok` and account
`Unknown` are joined and may be selected for fixed-space grants.
`joined` means native membership/local load, not HTTP authorization or gateway
readiness. Rejected/removed membership returns exit 2. Join result is
`{operation_id, space}`; status result is the space object; list result is
`{account_id, spaces:[]}`. An interrupted request without an observable membership
is uncertain and is never automatically submitted again.

## Integration credentials

```text
anytype auth apikey create 'stable application name' --json --expect-account FULL_ACCOUNT_ID --operation-id KEY_OPERATION --space FULL_SPACE_ID --read-write --include-secret
anytype auth apikey create 'stable application name' --json --expect-account FULL_ACCOUNT_ID --operation-id KEY_OPERATION --all-spaces --read-only --include-secret
anytype auth apikey revoke CREDENTIAL_ID --json --expect-account FULL_ACCOUNT_ID --operation-id REVOKE_OPERATION
```

Exactly one space scope (`--space` repeated or `--all-spaces`) and permission
(`--read-only` or `--read-write`) is required. Exact unique space names are also
accepted, resolved to full IDs. Application names remain unchanged across
rotation: Heart's creation/deletion ownership depends on the exact name, and the
verified limit is 128 UTF-8 bytes. Names are not operation identities.

Creation result contains `operation_id`, `account_id`, `credential_id` (Heart
AppHash), `application_name`, `scope`, `grant`, `created_at`, `expires_at`, and
`grant_verified:true`. Grant is `{all_spaces, space_ids, permission}` where
permission is `read` or `read_write`. `api_key` appears only with
`--include-secret` on private stdout. List returns `{account_id, credentials:[]}`
with the same non-secret credential fields; it never prints tokens.

Creation verifies the actual persisted grant by native readback. Recovery uses an
exact persisted key receipt or credential ID, never an application-name match.
The resolved space IDs and permission are persisted before creation and reused
on every replay, even if a selected space name changes or is reused. Old sent or
completed operation receipts without that resolved grant return uncertainty;
they do not recreate/revoke a key using a newly resolved name.
If the response/receipt was lost before identity could be recorded, even a unique
new same-name key is insufficient proof: return uncertainty, not a duplicate.
A provably newly created credential with wrong scope/grant is revoked by its exact
identity, with cleanup recorded and verified. Cleanup failure is explicit.
Externally changed keys from previously completed operations are not revoked.
Rotation retains all old credentials unless explicitly revoked by ID.

## Durable coordination and private storage

Mutations require an 8–128 character operation ID (`[A-Za-z0-9][A-Za-z0-9_-]*`).
Non-secret operation records in `automation/operations/` bind command intent,
instance paths, expected/derived account, effective listener/network, stage and
resource identity. Reusing an ID with different intent fails. One OS advisory
instance lock coordinates machine mutations, this version's human CLI mutations,
and service auto-login. External native RPC clients or older CLI versions do not
honor that lock; identity/session/observation checks detect incompatible account
transitions, but no distributed coordination is claimed.

Secrets are CLI-owned, using the existing `anytype-cli` system keyring when
available and owner-only atomic files in `automation/secrets/` otherwise.
Default lookup distinguishes an absent key from a denied or unavailable keyring;
the latter blocks account provisioning rather than claiming an empty instance.
For a fresh headless instance, explicitly set
`ANYTYPE_CLI_CREDENTIAL_BACKEND=private_file` on the provisioning invocation.
This selects private files and leaves unmarked legacy keyring entries untouched;
it does **not** prove those entries are absent. Use it only when deliberately
choosing file-backed credentials for this instance. The choice is saved on the
first credential write in `automation/credential-backend.json`, so later CLI
commands and service auto-login need no environment override. Existing per-secret
markers take precedence: system-keyring markers remain keyring-backed and cannot
be silently migrated by this selector or by keyring failures. Private-file
markers remain file-backed. Legacy config credentials still participate in
identity guards. Invalid selector values fail closed. Capabilities advertise
the selector, and account results report the actual credential backend.
On POSIX, files use mode 0600, file sync and directory sync after rename. On
macOS, opened-file ACLs are also checked: deny-only ACLs are accepted, granting
ACL entries are conservatively rejected. This applies to private stdin/stdout
and stored secrets. Inherited grants reject a temporary file before private
data is written; use a pipe or a private directory without granting ACLs.
macOS ACL checks require the CLI's normal cgo build. On
Windows, files are created with a protected ACL allowing only the current user
and LocalSystem, checked through the opened file's owner/DACL, flushed, and
replaced with `MoveFileEx` write-through. Windows' synthetic POSIX mode bits and
unsupported read-only directory flush are not used. The Windows storage helpers
and their ACL/round-trip tests are cross-compiled in this checkpoint; Windows
runtime acceptance still needs a Windows host.
Read-only inspection never tests keyring availability by writing an entry, creates
no directory, and does not migrate legacy storage. Legacy config credentials are
read compatibly. Human CLI and service auto-login share this credential store.
An unmarked legacy config credential remains readable when the keyring is
unavailable. Without a legacy value, that keyring error remains an error rather
than evidence of an empty instance. Existing markers and tombstones always win.
Logout commits tombstones, attempts physical deletion of owned keyring entries, and removes legacy
config credentials. Completed operation recovery receipts are retained separately
for explicit private recovery; logout does not erase those recovery receipts.
Treat the entire CLI config directory and keyring as sensitive backups.

Cancellation/failure never deletes accounts, leaves spaces, stops shared services,
or revokes older credentials. An uncertain mutation stays visible and requires
inspection/resume. There is no implicit cleanup merely because a caller exits.
