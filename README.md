# Anytype CLI

A command-line interface for interacting with [Anytype](https://github.com/anyproto/anytype-ts). This CLI embeds [anytype-heart](https://github.com/anyproto/anytype-heart) as the server, making it a complete, self-contained solution for developers to work with a headless Anytype instance.

## Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Usage](#usage)
  - [Running the Server](#running-the-server)
  - [Network Configuration](#network-configuration)
  - [Authentication](#authentication)
  - [API Keys](#api-keys)
  - [Space Management](#space-management)
- [Development](#development)
  - [Project Structure](#project-structure)
  - [Building from Source](#building-from-source)
- [Contribution](#contribution)

## Installation

Install the latest release with a single command:

```bash
/usr/bin/env bash -c "$(curl -fsSL https://raw.githubusercontent.com/anyproto/anytype-cli/HEAD/install.sh)"
```

## Quick Start

> [!IMPORTANT]
> The headless middleware requires a dedicated bot account, which you create using `anytype auth create`. This process generates an account key for authentication - mnemonic-based login is not supported. The bot account only has access to spaces it explicitly joins, keeping your data isolated and allowing you to easily revoke its access at any time from the desktop app.

Get up and running in just a few commands:

```bash
# Run the Anytype server
anytype serve

# Or install as a user service
anytype service install
anytype service start

# Create a new bot account
anytype auth create

# Join a space via invite link
anytype space join <invite-link>

# Verify the space was joined
anytype space list

# Create an API key for programmatic access
anytype auth apikey create "my-bot-api-key"
```

Once running, the API is available at `http://127.0.0.1:31012`. Use your API key to authenticate requests to the endpoints described on the [Developer Portal](https://developers.anytype.io). See [Network Configuration](#network-configuration) for remote access options.

## Usage

```
anytype <command> <subcommand> [flags]

Commands:
  auth        Manage authentication and accounts
  serve       Run anytype in foreground
  service     Manage anytype as a user service
  shell       Start interactive shell mode
  space       Manage spaces
  update      Update to the latest version
  version     Show version information

Examples:
  anytype serve                     # Run in foreground
  anytype service install           # Install as user service
  anytype service start             # Start the service
  anytype auth login                # Log in to your account
  anytype auth create               # Create a new account
  anytype space list                # List all available spaces

Use "anytype <command> --help" for more information about a command.
```

### Running the Server

The CLI embeds anytype-heart as the server that can be run in two ways:

#### 1. Interactive Mode (for development)

```bash
anytype serve
```

This runs the server in the foreground with logs output to stdout, similar to `ollama serve`.

#### 2. User Service (for production)

```bash
# Install as user service
anytype service install

# Start the service
anytype service start

# Check service status
anytype service status

# Stop the service
anytype service stop

# Uninstall the service
anytype service uninstall
```

The service management works across platforms:

- **macOS**: Uses User Agent (launchd)
- **Linux**: Uses systemd user service
- **Windows**: Uses Windows User Service

### Network Configuration

By default, the server binds to `127.0.0.1` (localhost only) on ports 31010-31012 and is not accessible from other machines. These ports are intentionally different from the Anytype desktop app (which uses 31007-31009), allowing both to run simultaneously on the same machine. Port 31012 is the main API endpoint used for HTTP requests.

| Port  | Service  | Description                 |
| ----- | -------- | --------------------------- |
| 31010 | gRPC     | gRPC server endpoint        |
| 31011 | gRPC-Web | gRPC-Web server endpoint    |
| 31012 | API      | HTTP API server endpoint ⭐ |


You can change the API listen address using `--listen-address` (e.g., `--listen-address 0.0.0.0:31012`). For remote access, you can also use a reverse proxy, SSH tunnel, or Docker port mapping to expose the local ports.

**Security note**: Always keep your API keys safe. If ports are exposed externally, third parties with your API key could gain unauthorized access to the spaces your headless instance has access to.

### Authentication

Manage your Anytype account and authentication:

```bash
# Create a new account
anytype auth create

# Log in to your account
anytype auth login

# Check authentication status
anytype auth status

# Log out and clear stored credentials
anytype auth logout
```

### API Keys

Manage API keys for programmatic access:

```bash
# Create a new API key
anytype auth apikey create "my-app"

# List all API keys
anytype auth apikey list

# Revoke an API key
anytype auth apikey revoke <key-id>
```

### Space Management

Work with Anytype spaces:

```bash
# List all available spaces
anytype space list

# Join a space
anytype space join <invite-link>

# Leave a space
anytype space leave <space-id>
```

## Development

### Project Structure

```
anytype-cli/
├── cmd/              # CLI commands
│   ├── auth/         # Authentication commands
│   ├── serve/        # Server command
│   ├── service/      # Service management
│   ├── space/        # Space management
│   └── ...
├── core/             # Core business logic
│   ├── grpcserver/   # Embedded gRPC server (anytype-heart)
│   ├── serviceprogram/ # Service implementation
│   └── ...
└── dist/             # Build output
```

### Building from Source

#### Prerequisites

- Go 1.24 or later
- Git
- Make
- C compiler (gcc or clang, for CGO)

#### Build Commands

```bash
# Clone the repository
git clone https://github.com/anyproto/anytype-cli.git
cd anytype-cli

# Build the CLI (automatically downloads tantivy library)
make build

# Install to ~/.local/bin
make install

# Run tests
go test ./...

# Run linting
make lint

# Cross-compile for all platforms
make cross-compile
```

#### Uninstall

```bash
# Remove installation from ~/.local/bin
make uninstall
```

## Contribution

Thank you for your desire to develop Anytype together!

❤️ This project and everyone involved in it is governed by the [Code of Conduct](https://github.com/anyproto/.github/blob/main/docs/CODE_OF_CONDUCT.md).

🧑‍💻 Check out our [contributing guide](https://github.com/anyproto/.github/blob/main/docs/CONTRIBUTING.md) to learn about asking questions, creating issues, or submitting pull requests.

🫢 For security findings, please email [security@anytype.io](mailto:security@anytype.io) and refer to our [security guide](https://github.com/anyproto/.github/blob/main/docs/SECURITY.md) for more information.

🤝 Follow us on [Github](https://github.com/anyproto) and join the [Contributors Community](https://github.com/orgs/anyproto/discussions).

---

Made by Any — a Swiss association 🇨🇭

Licensed under [MIT](./LICENSE.md).
