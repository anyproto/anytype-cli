package automation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/localstate"
)

type NativeService struct {
	Platform, Home, Executable string
	Run                        func(context.Context, string, ...string) ([]byte, error)
}

func NewNativeService() *NativeService {
	home, _ := os.UserHomeDir()
	exe, _ := os.Executable()
	return &NativeService{runtime.GOOS, home, exe, runProcess}
}
func runProcess(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

// A verified foreground runtime does not require an OS service adapter.
// Keep every other inspection error (including unavailable supported managers)
// so this cannot bypass an installed service's binding checks.
func (e *Engine) runtimeServiceStatus(ctx context.Context) (ServiceStatus, error) {
	s, err := e.Service.Status(ctx)
	var detail *Error
	if s.Manager == "unsupported" && errors.As(err, &detail) && detail.Code == "unsupported_service_platform" {
		return s, nil
	}
	return s, err
}

func (n *NativeService) Status(ctx context.Context) (ServiceStatus, error) {
	s := ServiceStatus{Name: "anytype", Health: "not_running"}
	switch n.Platform {
	case "darwin":
		s.Manager = "launchd-user"
		s.DefinitionPath = filepath.Join(n.Home, "Library/LaunchAgents/anytype.plist")
	case "linux":
		s.Manager = "systemd-user"
		s.DefinitionPath = filepath.Join(n.Home, ".config/systemd/user/anytype.service")
	default:
		s.Manager = "unsupported"
		return s, problem("unsupported_service_platform", "Native automation supports launchd user agents and systemd user services on this release.", 1)
	}
	data, err := os.ReadFile(s.DefinitionPath)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, problem("service_inspection_failed", "Cannot read the existing service definition.", 2)
	}
	s.Installed = true
	sum := sha256.Sum256(data)
	s.DefinitionHash = hex.EncodeToString(sum[:])
	var args []string
	effectiveOverrides := false
	if n.Platform == "darwin" {
		args, effectiveOverrides = plistArguments(data)
		out, err := n.Run(ctx, "launchctl", "list", "anytype")
		if err == nil {
			s.PID = launchdPID(string(out))
			s.Running = s.PID > 0
		}
	} else {
		// kardianos/service quotes each ExecStart token. Accept its generated format
		// only; arbitrary externally managed unit syntax is reported unverified.
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "ExecStart=") {
				args = parseUnitArgs(strings.TrimPrefix(line, "ExecStart="))
			}
		}
		out, err := n.Run(ctx, "systemctl", "--user", "show", "anytype.service", "--property=ActiveState,MainPID,FragmentPath,DropInPaths,Environment,LoadState,NeedDaemonReload,ExecStart")
		if err != nil {
			return s, problem("service_manager_unavailable", "The user service manager is unavailable.", 4)
		}
		properties := map[string]string{}
		for _, line := range strings.Split(string(out), "\n") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				properties[parts[0]] = parts[1]
			}
		}
		s.Running = properties["ActiveState"] == "active"
		s.PID, _ = strconv.Atoi(properties["MainPID"])
		effectiveOverrides = properties["FragmentPath"] != s.DefinitionPath || properties["DropInPaths"] != "" || properties["Environment"] != ""
		// The file can differ from the manager's cached definition. Never start
		// that cached command or silently reload an externally modified service.
		loadedPath, loadedArgs := loadedUnitCommand(properties["ExecStart"])
		if properties["LoadState"] != "loaded" || properties["NeedDaemonReload"] != "no" || len(args) == 0 || loadedPath != args[0] || !slices.Equal(loadedArgs, args) {
			effectiveOverrides = true
		}

	}
	if s.Running {
		s.Health = "running"
	}
	if len(args) > 0 {
		s.Executable = args[0]
	}
	s.ListenAddress = config.DefaultAPIAddress
	if len(args) >= 2 && args[1] == "serve" {
		recognized := true
		explicitAddress := false
		for i := 2; i < len(args); i++ {
			if args[i] == "--listen-address" && i+1 < len(args) {
				i++
				s.ListenAddress = args[i]
				explicitAddress = true
			} else {
				recognized = false
			}
		}
		if !explicitAddress {
			// The foreground serve command inherits the persisted address when
			// its service definition does not set --listen-address.
			var saved config.Config
			err := localstate.ReadJSON(filepath.Join(n.Home, config.AnytypeDirName, config.ConfigFileName), &saved)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				recognized = false
			} else {
				s.ListenAddress = config.ResolveAPIListenAddr("", false, saved.ApiListenAddr)
			}
		}
		// Data-path/environment overrides are not currently modeled per instance.
		if n.Platform == "darwin" && (bytes.Contains(data, []byte("EnvironmentVariables")) || bytes.Contains(data, []byte("RootDirectory")) || bytes.Contains(data, []byte("UserName"))) {
			recognized = false
		}
		if n.Platform == "linux" {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "EnvironmentFile=") {
					if line != "EnvironmentFile=-/etc/sysconfig/anytype" {
						recognized = false
					} else if _, err := os.Stat("/etc/sysconfig/anytype"); !errors.Is(err, os.ErrNotExist) {
						recognized = false
					}
				} else if strings.HasPrefix(line, "Environment=") || strings.HasPrefix(line, "RootDirectory=") || strings.HasPrefix(line, "User=") {
					recognized = false
				}
			}
		}
		if effectiveOverrides {
			recognized = false
		}
		executableMatches := sameExecutable(s.Executable, n.Executable)
		s.BindingVerified = recognized && executableMatches && validateAddress(s.ListenAddress) == nil
	}
	return s, nil
}

// loadedUnitCommand accepts the single ExecStart structure printed by
// systemctl show. Unrecognized/ambiguous escaping or multiple commands remain
// unverified; there is no shell evaluation or supervisor mutation here.
func loadedUnitCommand(value string) (string, []string) {
	if !strings.HasPrefix(value, "{ ") || !strings.HasSuffix(value, " }") {
		return "", nil
	}
	inner := value[2 : len(value)-2]
	if strings.ContainsAny(inner, "{}\n") {
		return "", nil
	}
	fields := map[string]string{}
	for _, field := range strings.Split(inner, " ; ") {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			return "", nil
		}
		if _, duplicate := fields[parts[0]]; duplicate {
			return "", nil
		}
		fields[parts[0]] = parts[1]
	}
	if fields["ignore_errors"] != "no" {
		return "", nil
	}
	return fields["path"], parseUnitArgs(fields["argv[]"])
}
func sameExecutable(a, b string) bool {
	aa, ea := filepath.EvalSymlinks(a)
	bb, eb := filepath.EvalSymlinks(b)
	return ea == nil && eb == nil && aa == bb
}
func plistArguments(data []byte) (args []string, unsupported bool) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	found := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return args, unsupported
		}
		if err != nil {
			return nil, true
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "key" {
			var key string
			if decoder.DecodeElement(&key, &start) != nil {
				return nil, true
			}
			// launchd Program overrides the executable in ProgramArguments.
			// This release only verifies the service library's no-override form.
			if key == "Program" || key == "ProgramArguments" && args != nil {
				unsupported = true
			}
			found = key == "ProgramArguments"
			continue
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "array" && found {
			var array struct {
				Values []string `xml:"string"`
			}
			if decoder.DecodeElement(&array, &start) != nil {
				return nil, true
			}
			args = array.Values
			found = false
		}
	}
}
func parseUnitArgs(value string) []string {
	// The service library uses Go-style double quoted arguments. Do not evaluate
	// shell syntax, specifiers, environment expansion, or systemd command prefixes.
	var result []string
	for len(strings.TrimSpace(value)) > 0 {
		value = strings.TrimSpace(value)
		if value[0] != '"' {
			i := strings.IndexAny(value, " \t")
			if i < 0 {
				i = len(value)
			}
			token := value[:i]
			if strings.ContainsAny(token, "%$\\'`") {
				return nil
			}
			result = append(result, token)
			value = value[i:]
			continue
		}
		end := 1
		for end < len(value) {
			if value[end] == '\\' {
				end += 2
				continue
			}
			if value[end] == '"' {
				break
			}
			end++
		}
		if end >= len(value) {
			return nil
		}
		token, err := strconv.Unquote(value[:end+1])
		if err != nil || strings.ContainsAny(token, "%$") {
			return nil
		}
		result = append(result, token)
		value = value[end+1:]
	}
	return result
}
func launchdPID(output string) int {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "\"PID\"") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				pid, _ := strconv.Atoi(strings.Trim(strings.TrimSpace(parts[1]), ";"))
				return pid
			}
		}
	}
	return 0
}
func (n *NativeService) child(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, n.Executable, args...)
	cmd.Env = append(os.Environ(), "ANYTYPE_CLI_LOCK_PARENT_PID="+strconv.Itoa(os.Getpid()))
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if cmd.Run() != nil {
		return problem("service_action_unavailable", "The service action did not complete. Inspect status and resume the same operation ID.", 4)
	}
	return nil
}
func (n *NativeService) Install(ctx context.Context, addr string) error {
	return n.child(ctx, "--no-update-check", "service", "install", "--listen-address", addr)
}
func (n *NativeService) Start(ctx context.Context) error {
	return n.child(ctx, "--no-update-check", "service", "start")
}
func (e *Engine) serviceMutation(ctx context.Context, r Request, op *operation, retry bool) Envelope {
	c, err := e.config()
	if err != nil {
		return Failure(err)
	}
	svc, err := e.Service.Status(ctx)
	if err != nil {
		return Failure(err)
	}
	addr := config.ResolveAPIListenAddr("", false, c.ApiListenAddr)
	if r.ListenAddress != "" {
		addr = r.ListenAddress
	}
	if err = validateAddress(addr); err != nil {
		return Failure(err)
	}
	if c.AccountId != "" && addr != config.ResolveAPIListenAddr("", false, c.ApiListenAddr) {
		return Failure(problem("settings_conflict", "Service provisioning cannot change an existing account's listener.", 2))
	}
	if os.Getenv("DATA_PATH") != "" {
		return Failure(problem("unsupported_service_binding", "Automatic service management does not support DATA_PATH overrides. Use an externally managed instance.", 1))
	}
	if svc.Installed {
		if !svc.BindingVerified || svc.ListenAddress != addr {
			return Failure(problem("service_binding_conflict", "An existing service has a different or unverified binding; it will not be replaced or reinstalled.", 2))
		}
		if op.ResourceId != "" && op.ResourceId != svc.DefinitionHash {
			return Failure(problem("service_binding_conflict", "The service definition changed during this operation.", 2))
		}
	} else if r.Command == "service start" {
		return Failure(problem("service_not_installed", "Install the service explicitly before starting it.", 2))
	}
	changed := false
	if r.Command == "service install" && !svc.Installed {
		// Do not install over an independently running native server.
		if _, liveErr := e.live(ctx); liveErr == nil {
			return Failure(problem("external_service_running", "A native server is already running without this service definition. Reuse it or manage it externally.", 2))
		} else {
			var detail *Error
			if !errors.As(liveErr, &detail) || detail.Code != "rpc_unavailable" {
				return Failure(liveErr)
			}
		}
		if retry && op.Stage != "prepared" {
			return Failure(uncertain())
		}
		if err = e.saveOp(op, "install_sent"); err != nil {
			return Failure(err)
		}
		if err = e.Service.Install(ctx, addr); err != nil {
			return Failure(err)
		}
		changed = true
	}
	if r.Command == "service start" && !svc.Running {
		op.ResourceId = svc.DefinitionHash
		if err = e.saveOp(op, "start_sent"); err != nil {
			return Failure(err)
		}
		if err = e.Service.Start(ctx); err != nil {
			return Failure(err)
		}
		changed = true
	}
	observed, err := e.Service.Status(ctx)
	if err != nil {
		return Failure(err)
	}
	if !observed.Installed || !observed.BindingVerified || observed.ListenAddress != addr {
		return Failure(problem("service_binding_conflict", "Service installation could not be verified; inspect it before resuming.", 2))
	}
	if r.Command == "service start" && !observed.Running {
		return Failure(problem("service_start_pending", "The supervisor has not yet reported the service running. Retry status.", 4))
	}
	op.ResourceId = observed.DefinitionHash
	if err = e.saveOp(op, "complete"); err != nil {
		return Failure(err)
	}
	return Success(map[string]any{"operation_id": op.Id, "service": observed, "api_ready": false, "next_probe": "inspect; API availability is independent of installation/start"}, changed)
}
