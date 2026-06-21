# Self-Hosted Network Setup

This guide covers connecting to a self-hosted [any-sync](https://github.com/anyproto/any-sync) network. For general installation and usage, see [README.md](README.md).

## Prerequisites

- Anytype CLI installed (see [README.md#installation](README.md#installation))
- Self-hosted any-sync network running
- Network configuration YAML from your deployment

## Network Configuration

### 1. Create Network Config File

Save your network configuration from your any-sync deployment:

```yaml
# ~/.config/anytype/network.yml
networkId: YOUR_NETWORK_ID
nodes:
  - peerId: 12D3KooW...
    addresses:
      - your.server.com:33021
    types:
      - coordinator
  - peerId: 12D3KooW...
    addresses:
      - your.server.com:33022
    types:
      - consensus
  - peerId: 12D3KooW...
    addresses:
      - your.server.com:33020
    types:
      - tree
  - peerId: 12D3KooW...
    addresses:
      - your.server.com:33023
    types:
      - file
```

### 2. Start Server

```bash
anytype serve
```

### 3. Create Account with Network Config

Use the `--network-config` flag when creating your account:

```bash
anytype auth create my-bot --network-config ~/.config/anytype/network.yml
```

Save the account key - it's your only authentication credential. The network config path is saved to `~/.anytype/config.json` for future operations.

## Joining Spaces

Generate an invite link from the Anytype app connected to your self-hosted network, then join:

```bash
anytype space join "<invite-link>"
```

The CLI automatically uses the cached network ID from your config. Supported invite formats:
- `https://<host>/<cid>#<key>` (web invite)
- `anytype://invite/?cid=<cid>&key=<key>` (app deep link)

## Troubleshooting

**`network id mismatch`**
Re-create your account with `--network-config` pointing to the correct YAML file.

**`DeadlineExceeded`**
Check network connectivity to your self-hosted nodes.

**`no ns peers configured`** / **`membership status`**
These warnings are expected on self-hosted networks (naming service and membership are only available on the Anytype Network).

## Configuration Files

Self-hosted networks use these additional config entries:

| File | Field | Purpose |
|------|-------|---------|
| `~/.anytype/config.json` | `networkConfigPath` | Path to your network YAML |
| `~/.anytype/config.json` | `networkId` | Cached network ID for space joins |
| `~/.config/anytype/network.yml` | - | Your self-hosted network nodes |

For other configuration details, see [README.md](README.md).
