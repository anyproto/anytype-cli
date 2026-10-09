// Package automation implements the versioned, noninteractive CLI contract.
package automation

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/localstate"
	"github.com/anyproto/anytype-cli/core/runtimeinfo"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const SchemaVersion = 1

type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Exit      int    `json:"-"`
}

func (e *Error) Error() string                      { return e.Message }
func problem(code, message string, exit int) *Error { return &Error{code, message, exit == 4, exit} }
func uncertain() *Error {
	return problem("operation_uncertain", "The operation may have completed. Inspect its recorded result; do not repeat it with a new operation ID.", 2)
}

type Envelope struct {
	SchemaVersion int    `json:"schema_version"`
	State         string `json:"state"`
	Changed       bool   `json:"changed"`
	Result        any    `json:"result"`
	NextAction    string `json:"next_action"`
	Error         *Error `json:"error,omitempty"`
}

func Success(result any, changed bool) Envelope {
	return Envelope{SchemaVersion: SchemaVersion, State: "ready", Changed: changed, Result: result}
}
func Failure(err error) Envelope {
	var e *Error
	if !errors.As(err, &e) {
		e = problem("internal_error", "The CLI could not complete the operation; no credentials are included in this diagnostic.", 1)
	}
	state := "needs_input"
	if e.Exit == 4 {
		state = "unavailable"
	}
	if e.Exit == 1 {
		state = "incompatible"
	}
	return Envelope{SchemaVersion: SchemaVersion, State: state, NextAction: e.Message, Error: e}
}
func (e Envelope) ExitCode() int {
	if e.Error != nil {
		return e.Error.Exit
	}
	if e.State == "pending_approval" || e.State == "syncing" || e.State == "request_sent" {
		return 3
	}
	return 0
}

type Request struct {
	Command         string
	Instance        string
	OperationId     string
	ExpectedAccount string
	Name            string
	Secret          string
	IncludeSecret   bool
	NetworkConfig   string
	ListenAddress   string
	SpaceId         string
	CredentialId    string
	Grant           core.GrantFlags
}

type Account struct {
	Id                string `json:"id,omitempty"`
	DisplayName       string `json:"display_name,omitempty"`
	Authenticated     bool   `json:"authenticated"`
	Observation       string `json:"observation"`
	CredentialStorage string `json:"credential_storage,omitempty"`
}

type ServiceStatus struct {
	Name            string `json:"name"`
	Manager         string `json:"manager"`
	DefinitionPath  string `json:"definition_path,omitempty"`
	DefinitionHash  string `json:"definition_hash,omitempty"`
	Executable      string `json:"executable,omitempty"`
	Installed       bool   `json:"installed"`
	Running         bool   `json:"running"`
	BindingVerified bool   `json:"binding_verified"`
	ListenAddress   string `json:"listen_address,omitempty"`
	RuntimePID      int    `json:"runtime_pid,omitempty"`
	PID             int    `json:"pid,omitempty"`
	Health          string `json:"health"`
}

type ServiceManager interface {
	Status(context.Context) (ServiceStatus, error)
	Install(context.Context, string) error
	Start(context.Context) error
}

type Secrets interface {
	Get(string) (string, string, error)
	Set(string, string) (string, error)
}

type Engine struct {
	Client          service.ClientCommandsClient
	ConfigPath      string
	DataPath        string
	WorkPath        string
	StateDir        string
	RuntimePath     string
	Secrets         Secrets
	Service         ServiceManager
	Live            func(context.Context) (*runtimeinfo.Snapshot, error)
	IdentityFromKey func(string) (string, error)
	Timeout         time.Duration
}

func (e *Engine) config() (*config.Config, error) {
	c := new(config.Config)
	err := localstate.ReadJSON(e.ConfigPath, c)
	if errors.Is(err, errMissing) {
		return c, nil
	}
	if err != nil {
		return nil, problem("config_unreadable", "Cannot read the selected instance configuration.", 2)
	}
	return c, nil
}

func ValidateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) {
		return "", problem("invalid_name", "An explicit nonempty UTF-8 display name is required.", 2)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", problem("invalid_name", "Display names cannot contain control characters.", 2)
		}
	}
	// Heart's AccountCreate/profile SetDetails path has no name-specific maximum.
	// This is an explicit CLI input bound, not an invented Heart limit.
	if len(name) > 4096 {
		return "", problem("invalid_name", "The CLI accepts display names up to 4096 UTF-8 bytes.", 2)
	}
	return name, nil
}

func capabilities() map[string]any {
	return map[string]any{
		"instance_selection": []string{"default"}, "isolated_instances": false, "account_rename": false,
		"account_create": true, "account_login": true, "space_join": true, "scoped_api_keys": true,
		"operation_recovery": "durable_checkpoints_with_explicit_uncertainty", "coordination": "same-version CLI commands and service auto-login",
		"credential_storage":                []string{"system_keyring", "private_file"},
		"credential_backend_selection":      map[string]any{"environment": "ANYTYPE_CLI_CREDENTIAL_BACKEND", "values": []string{"private_file"}, "default": "system_keyring_with_private_file_fallback", "existing_markers_preserved": true},
		"limits":                            map[string]any{"display_name_max_bytes": 4096, "display_name_limit_source": "CLI input bound; Heart has no name-specific bound", "application_name_max_bytes": domain.MaxIntegrationNameLen},
		"embedded_http_api":                 map[string]any{"versions": []string{"v1", "v2"}, "whoami": "/v2/auth/whoami", "readiness": "must_probe_with_integration_key"},
		"embedded_http_mcp":                 map[string]any{"path": "/mcp/full", "other_paths": []string{"/mcp/small", "/mcp/large"}, "transport": "streamable_http", "stateless": true, "authentication": "bearer", "token_source": "same_api_key", "same_grant": true, "protocol_versions": []string{"2025-06-18", "2025-11-25"}, "auth_whoami_tool": false, "readiness": "not_probed"},
		"invitation_expiry_distinguishable": false,
	}
}

func apiOrigin(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	if host == "0.0.0.0" || host == "::" || host == "" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}
func (e *Engine) binding() string {
	return filepath.Clean(e.ConfigPath) + "|" + filepath.Clean(e.DataPath)
}
func grantJSON(g *model.AccountAuthAppGrant) any {
	if g == nil {
		return nil
	}
	perm := "read"
	if g.Perm == model.AccountAuthAppGrant_ReadWrite {
		perm = "read_write"
	}
	return map[string]any{"all_spaces": g.AllSpaces, "space_ids": g.SpaceIds, "permission": perm}
}
