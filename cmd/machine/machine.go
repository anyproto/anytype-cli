// Package machine is an additive parser for the existing command paths. The
// human command implementations are never invoked to perform RPC mutations.
package machine

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	"github.com/anyproto/anytype-cli/core/automation"
	"github.com/anyproto/anytype-cli/core/localstate"
	"github.com/spf13/cobra"
)

func Requested(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--json" || strings.HasPrefix(arg, "--json=") {
			return true
		}
	}
	return false
}

type Runner interface {
	Run(context.Context, automation.Request) automation.Envelope
}

func Execute(ctx context.Context, args []string, in io.Reader, out io.Writer, runner Runner) int {
	var request automation.Request
	var machineMode, noninteractive, noUpdate bool
	var timeout time.Duration
	var envelope automation.Envelope
	written := false
	root := &cobra.Command{Use: "anytype", Short: "Anytype automation (schema version 1)", SilenceErrors: true, SilenceUsage: true}
	root.SetArgs(args)
	root.SetIn(in)
	var discard bytes.Buffer
	root.SetOut(&discard)
	root.SetErr(&discard)
	root.PersistentFlags().BoolVar(&machineMode, "json", false, "Return a versioned JSON envelope; never prompt")
	root.PersistentFlags().BoolVar(&noninteractive, "non-interactive", false, "Never prompt (implied by --json)")
	root.PersistentFlags().BoolVar(&noUpdate, "no-update-check", true, "No update checks in machine mode")
	root.PersistentFlags().StringVar(&request.Instance, "instance", "default", "Native instance (only default is supported)")
	root.PersistentFlags().StringVar(&request.ExpectedAccount, "expect-account", "", "Full expected account ID; none guards an empty instance")
	root.PersistentFlags().DurationVar(&timeout, "timeout", 45*time.Second, "Whole-command deadline, 1s to 5m")
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		envelope = automation.Success(map[string]any{"help": cmd.UsageString()}, false)
		written = true
	})
	add := func(parent *cobra.Command, use, path string, mutating bool) *cobra.Command {
		cmd := &cobra.Command{Use: use, Short: path, Args: cobra.NoArgs}
		if mutating {
			cmd.Flags().StringVar(&request.OperationId, "operation-id", "", "Durable operation ID; reuse it on retry")
		}
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			request.Command = path
			if len(args) == 1 {
				switch path {
				case "auth create", "auth apikey create":
					request.Name = args[0]
				case "auth apikey revoke":
					request.CredentialId = args[0]
				case "space status":
					request.SpaceId = args[0]
				}
			}
			if timeout < time.Second || timeout > 5*time.Minute {
				envelope = automation.Failure(&automation.Error{Code: "invalid_timeout", Message: "The timeout must be between 1s and 5m.", Exit: 2})
				written = true
				return nil
			}
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			if request.IncludeSecret && !privateChannel(out) {
				envelope = automation.Failure(&automation.Error{Code: "private_output_required", Message: "Capture --include-secret output through a pipe or an owner-only regular file.", Exit: 2})
				written = true
				return nil
			}
			keyStdin, _ := cmd.Flags().GetBool("key-stdin")
			inviteStdin, _ := cmd.Flags().GetBool("invite-stdin")
			if keyStdin || inviteStdin {
				if !privateChannel(in) {
					envelope = automation.Failure(&automation.Error{Code: "private_input_required", Message: "Supply stdin through a pipe or an owner-only regular file; machine mode never prompts.", Exit: 2})
					written = true
					return nil
				}
				type inputResult struct {
					data []byte
					err  error
				}
				ch := make(chan inputResult, 1)
				go func() { data, err := io.ReadAll(io.LimitReader(in, 65537)); ch <- inputResult{data, err} }()
				select {
				case result := <-ch:
					if result.err != nil || len(result.data) > 65536 {
						envelope = automation.Failure(&automation.Error{Code: "invalid_private_input", Message: "Private input could not be read or exceeded 65536 bytes.", Exit: 2})
						written = true
						return nil
					}
					request.Secret = strings.TrimSpace(string(result.data))
				case <-ctx.Done():
					envelope = automation.Failure(&automation.Error{Code: "private_input_timeout", Message: "Private input timed out.", Exit: 4, Retryable: true})
					written = true
					return nil
				}
			}
			envelope = runner.Run(ctx, request)
			written = true
			return nil
		}
		parent.AddCommand(cmd)
		return cmd
	}
	add(root, "inspect", "inspect", false)
	add(root, "capabilities", "capabilities", false)
	service := &cobra.Command{Use: "service", Short: "Inspect and provision the default user service"}
	root.AddCommand(service)
	add(service, "status", "service status", false)
	for _, name := range []string{"install", "start"} {
		cmd := add(service, name, "service "+name, true)
		cmd.Flags().StringVar(&request.ListenAddress, "listen-address", "", "Explicit API listen address; existing binding is preserved")
	}
	auth := &cobra.Command{Use: "auth", Short: "Inspect or provision an account"}
	root.AddCommand(auth)
	add(auth, "status", "auth status", false)
	create := add(auth, "create <name>", "auth create", true)
	create.Args = cobra.ExactArgs(1)
	login := add(auth, "login", "auth login", true)
	login.Flags().Bool("key-stdin", false, "Read the private account key from stdin")
	for _, cmd := range []*cobra.Command{create, login} {
		cmd.Flags().StringVar(&request.ListenAddress, "listen-address", "", "Explicit API listen address")
		cmd.Flags().StringVar(&request.NetworkConfig, "network-config", "", "Network configuration file (preserved for existing accounts)")
		cmd.Flags().BoolVar(&request.IncludeSecret, "include-secret", false, "Include recovery credential in private output")
	}
	rename := add(auth, "rename <name>", "auth rename", false)
	rename.Args = cobra.ExactArgs(1)
	keys := &cobra.Command{Use: "apikey", Short: "Manage integration credentials"}
	auth.AddCommand(keys)
	add(keys, "list", "auth apikey list", false)
	keyCreate := add(keys, "create <application-name>", "auth apikey create", true)
	keyCreate.Args = cobra.ExactArgs(1)
	keyCreate.Flags().StringArrayVar(&request.Grant.Spaces, "space", nil, "Full space ID or exact unique name (repeatable)")
	keyCreate.Flags().BoolVar(&request.Grant.AllSpaces, "all-spaces", false, "Grant access to all current and future spaces")
	keyCreate.Flags().BoolVar(&request.Grant.ReadOnly, "read-only", false, "Read-only permission")
	keyCreate.Flags().BoolVar(&request.Grant.ReadWrite, "read-write", false, "Read and write permission")
	keyCreate.Flags().BoolVar(&request.IncludeSecret, "include-secret", false, "Return the verified credential secret to the private caller")
	revoke := add(keys, "revoke <credential-id>", "auth apikey revoke", true)
	revoke.Args = cobra.ExactArgs(1)
	spaces := &cobra.Command{Use: "space", Short: "Native membership; HTTP usability is probed separately"}
	root.AddCommand(spaces)
	add(spaces, "list", "space list", false)
	spaceStatus := add(spaces, "status <space-id>", "space status", false)
	spaceStatus.Args = cobra.ExactArgs(1)
	join := add(spaces, "join", "space join", true)
	join.Flags().Bool("invite-stdin", false, "Read a private invitation URL from stdin")
	join.Flags().StringVar(&request.SpaceId, "space-id", "", "Require this invitation target (optional)")
	if err := root.ExecuteContext(ctx); err != nil {
		envelope = automation.Failure(&automation.Error{Code: "invalid_arguments", Message: "Invalid machine command or arguments. Use --json --help for the supported interface; secrets must use stdin.", Exit: 2})
		written = true
	}
	if !written {
		envelope = automation.Success(map[string]any{"help": root.UsageString()}, false)
	}
	if err := json.NewEncoder(out).Encode(envelope); err != nil {
		return 1
	}
	return envelope.ExitCode()
}
func privateChannel(v any) bool {
	f, ok := v.(*os.File)
	if !ok {
		return true
	} // injected streams are owned by the caller
	info, err := f.Stat()
	if err != nil {
		return false
	}
	if info.Mode().IsRegular() {
		return localstate.CheckPrivateFile(f) == nil
	}
	return info.Mode()&os.ModeNamedPipe != 0 || info.Mode()&os.ModeSocket != 0
}
