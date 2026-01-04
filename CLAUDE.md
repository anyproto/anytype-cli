# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is the Anytype CLI, a Go-based command-line interface for interacting with Anytype. It includes an embedded gRPC server (from anytype-heart) making it a complete, self-contained binary that provides both server and CLI functionality. The CLI embeds the anytype-heart middleware server directly, eliminating the need for separate server installation or daemon processes.

## Build Commands

```bash
# Build the CLI (includes embedded server, downloads tantivy library automatically)
make build

# Build with custom self-hosted network configuration
make build ANY_SYNC_NETWORK=/path/to/heart.yml

# Install to ~/.local/bin (user installation)
make install

# Uninstall from ~/.local/bin
make uninstall

# Clean tantivy libraries
make clean-tantivy

# Run linting
make lint

# Cross-compile for all platforms
make cross-compile

# Build for specific platform
make build-darwin-amd64
make build-linux-amd64
make build-windows-amd64
```

### Build Requirements

- **CGO**: The build requires CGO_ENABLED=1 due to tantivy (full-text search library) dependencies
- **Tantivy Library**: Automatically downloaded for your platform during `make build`
- **C Compiler**: Required for CGO and linking tantivy library (clang on macOS, gcc on Linux, mingw on Windows)
- **Go 1.20+**: Required for building the project

### Self-Hosted Network Configuration

To build the CLI for use with self-hosted sync nodes:

1. **Generate Network Configuration**: Use `any-sync-network` tool or your self-hosted setup to generate a `heart.yml` configuration file
2. **Build with Custom Network**: Run `make build ANY_SYNC_NETWORK=/path/to/heart.yml`
3. **Install and Use**: The resulting binary will connect to your self-hosted sync nodes instead of the default Anytype network

The network configuration is compiled into the binary at build time and cannot be changed at runtime.

## Development Workflow

1. **Initial Setup**:
   ```bash
   make build  # Build the CLI with embedded server
   ```

2. **Running the Application**:
   ```bash
   # Run server interactively (for development)
   ./dist/anytype serve

   # Or install as user service
   ./dist/anytype service install
   ./dist/anytype service start
   ```

3. **Code Formatting and Linting**:
   ```bash
   go fmt ./...
   go vet ./...
   make lint  # Uses golangci-lint v2.2.1
   ```

4. **Testing**:
   ```bash
   # Run all tests
   make test
   
   # Run tests with coverage
   go test -cover ./...
   
   # Run tests for specific package
   go test ./core/...
   go test ./cmd/...
   
   # Run specific test
   go test -run TestValidateAccountKey ./core
   
   # Run tests with verbose output
   go test -v ./...
   ```

## Architecture Overview

### Embedded Server Architecture
The CLI embeds the anytype-heart gRPC server directly, creating a self-contained binary. This eliminates the need for separate server installation or management. The server runs either interactively (`anytype serve`) or as a user service.

### Command Structure (`/cmd/`)
- Uses Cobra framework for CLI commands
- Each command group has its own directory:
  - `auth/`: Authentication commands (login, logout, status)
    - `apikey/`: API key management (create, list, revoke)
  - `serve/`: Run the embedded Anytype server in foreground
  - `service/`: System service management (install, uninstall, start, stop, restart, status)
  - `space/`: Space management operations
  - `shell/`: Interactive shell mode
  - `update/`: Self-update functionality
  - `version/`: Version information
- `root.go` registers all commands

### Core Logic (`/core/`)
- `client.go`: gRPC client singleton using sync.Once for lazy initialization
- `auth.go`: Authentication logic with keyring integration
- `space.go`: Space management operations
- `stream.go`: Event streaming with EventReceiver and message batching (cheggaaa/mb)
- `keyring.go`: Secure credential storage using system keyring
- `apikey.go`: API key generation and management
- `config/`: Configuration management with constants
- `serviceprogram/`: Cross-platform service implementation (Windows Service, macOS launchd, Linux systemd)
- `grpcserver/`: Embedded gRPC server implementation

## Key Dependencies

- `github.com/anyproto/anytype-heart v0.42.3`: The embedded middleware server (provides all Anytype functionality)
- `github.com/spf13/cobra v1.9.1`: CLI framework for command structure
- `google.golang.org/grpc v1.74.2`: gRPC client-server communication
- `github.com/zalando/go-keyring v0.2.6`: Secure credential storage in system keyring
- `github.com/cheggaaa/mb/v3 v3.0.2`: Message batching queue for event handling
- `github.com/kardianos/service v1.2.4`: Cross-platform user service management
- `github.com/anyproto/tantivy-go v1.0.4`: Full-text search capabilities (requires CGO)

## Important Notes

1. **Service Architecture**: The CLI includes an embedded gRPC server that runs as a user service or interactively
2. **Cross-Platform Service**: Works on Windows (User Service), macOS (User Agent/launchd), Linux (systemd user service)
3. **Keyring Integration**: Authentication tokens are stored securely in the system keyring
4. **Port Configuration**:
   - gRPC: localhost:31007
   - gRPC-Web: localhost:31008
   - API: localhost:31009
5. **Event Streaming**: Uses server-sent events for real-time updates with message batching
6. **Version Management**: Version info is injected at build time via ldflags
7. **Self-Updating**: The CLI can update itself using the `anytype update` command
8. **API Keys**: Support for generating API keys for programmatic access
9. **Test Infrastructure**: Basic test structure with testify framework for assertions

## Common Development Tasks

### Adding a New Command
1. Create a new directory under `/cmd/` for your command group
2. Create a file named after the command (e.g., `config.go` for config command) with a `NewXxxCmd()` function that returns `*cobra.Command`
3. Create subdirectories for each subcommand with their own files
4. Import subcommands with aliases matching the directory name
5. Register the command in `/cmd/root.go` using `NewXxxCmd()`
6. Implement core logic in `/core/` if needed

Directory structure follows the subcommand pattern:
```
cmd/
├── config/
│   ├── config.go      # Main command file (not cmd.go)
│   ├── get/
│   │   └── get.go     # Subcommand with NewGetCmd()
│   ├── set/
│   │   └── set.go     # Subcommand with NewSetCmd()
│   └── reset/
│       └── reset.go   # Subcommand with NewResetCmd()
```

Example main command file:
```go
// cmd/config/config.go
package config

import (
    "github.com/spf13/cobra"
    
    configGetCmd "github.com/anyproto/anytype-cli/cmd/config/get"
    configSetCmd "github.com/anyproto/anytype-cli/cmd/config/set"
    configResetCmd "github.com/anyproto/anytype-cli/cmd/config/reset"
)

func NewConfigCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "config <command>",
        Short: "Manage configuration",
    }
    
    cmd.AddCommand(configGetCmd.NewGetCmd())
    cmd.AddCommand(configSetCmd.NewSetCmd())
    cmd.AddCommand(configResetCmd.NewResetCmd())
    
    return cmd
}
```

### Working with the gRPC Client
- Client singleton is initialized lazily in `core/client.go`
- Use `core.GRPCCall()` for authenticated calls
- Use `core.GRPCCallNoAuth()` for unauthenticated calls
- Connection errors automatically trigger reconnection attempts

### Working with the Service
- Service is managed via the `anytype service` command
- Service program implementation is in `core/serviceprogram/`
- Supports both interactive mode (`anytype serve`) and user service installation
- Service logs are written to log files in the user's home directory (~/.anytype/logs/)

### Error Handling
- Client connection errors are handled in `core/client.go`
- Server startup errors are managed in `core/serviceprogram/serviceprogram.go`
- Use standard Go error wrapping with `fmt.Errorf("context: %w", err)`
- Display user-friendly errors with `output.Error()`

### API Key Management
- API keys are created and managed by the server via gRPC APIs
- The CLI provides commands to create, list, and revoke API keys
- Keys are generated server-side and can be used for programmatic access
- Keys are stored in the system keyring alongside tokens

### Testing Strategy
- **Unit Tests**: Test individual functions and logic in isolation
- **Command Tests**: Test Cobra command execution and flags
- **Simple Tests**: Keep tests minimal and focused on real logic
- **Test Files**: Follow Go convention with `*_test.go` in same package
- **Table-driven tests**: Use for testing multiple scenarios

## Code Style Guidelines

### Naming Conventions
- **Use `Id` instead of `ID`**: Throughout the codebase, prefer `Id` over `ID` for consistency (e.g., `UserId`, `SpaceId`, `ObjectId`)