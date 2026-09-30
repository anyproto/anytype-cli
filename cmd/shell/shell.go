package shell

import (
	"errors"
	"io"
	"strings"
	"unicode"

	"github.com/chzyer/readline"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/anyproto/anytype-cli/core/output"
	"github.com/anyproto/anytype-cli/core/updatecheck"
)

func NewShellCmd(rootCmd *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "shell",
		Short: "Start interactive shell mode",
		Long:  "Launch an interactive shell where you can run Anytype commands without the 'anytype' prefix. Type 'exit' to quit.",
		RunE: func(cmd *cobra.Command, args []string) error {
			updatecheck.Disable()
			output.Info("Starting Anytype interactive shell. Type 'exit' to quit.")
			return runShell(rootCmd)
		},
	}
}

func runShell(rootCmd *cobra.Command) error {
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          ">>> ",
		HistoryLimit:    1000,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		AutoComplete:    buildCompleter(rootCmd),
	})
	if err != nil {
		return output.Error("Failed to initialize readline: %w", err)
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if errors.Is(err, readline.ErrInterrupt) {
			if len(line) == 0 {
				output.Info("Use 'exit' or 'quit' to leave the shell")
				continue
			}
		} else if err == io.EOF {
			return nil
		} else if err != nil {
			output.Warning("Error reading input: %v", err)
			continue
		}

		line = strings.TrimSpace(line)

		if line == "exit" || line == "quit" {
			return nil
		}

		if line == "" {
			continue
		}

		if err := executeLine(rootCmd, line); errors.Is(err, errAlreadyInShell) {
			output.Warning("Already in shell mode. Type 'exit' or 'quit' to leave.")
		} else if err != nil {
			output.Warning("Command error: %v", err)
		}
	}
}

var errAlreadyInShell = errors.New("already in shell mode")

// executeLine runs one shell line against the shared command tree. Flag values
// are reset first: cobra keeps them between executions, so an earlier
// command's --space or --read-only would otherwise leak into the next one.
func executeLine(rootCmd *cobra.Command, line string) error {
	args, err := splitLine(line)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return nil
	}
	if args[0] == "shell" {
		return errAlreadyInShell
	}

	resetFlags(rootCmd)
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

func resetFlags(cmd *cobra.Command) {
	reset := func(flag *pflag.Flag) {
		if slice, ok := flag.Value.(pflag.SliceValue); ok {
			_ = slice.Replace(nil)
		} else {
			_ = flag.Value.Set(flag.DefValue)
		}
		flag.Changed = false
	}
	cmd.Flags().VisitAll(reset)
	cmd.PersistentFlags().VisitAll(reset)
	for _, sub := range cmd.Commands() {
		resetFlags(sub)
	}
}

// splitLine splits a shell line into arguments. Whitespace separates
// arguments; single quotes keep text literally; double quotes keep whitespace
// and allow \" and \\ escapes.
func splitLine(line string) ([]string, error) {
	var args []string
	var current strings.Builder
	inArg := false
	var quote rune
	escaped := false

	for _, r := range line {
		switch {
		case escaped:
			if r != '"' && r != '\\' {
				current.WriteRune('\\')
			}
			current.WriteRune(r)
			escaped = false
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case quote == '"':
			switch r {
			case '"':
				quote = 0
			case '\\':
				escaped = true
			default:
				current.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inArg = true
		case unicode.IsSpace(r):
			if inArg {
				args = append(args, current.String())
				current.Reset()
				inArg = false
			}
		default:
			current.WriteRune(r)
			inArg = true
		}
	}

	if quote != 0 || escaped {
		return nil, errors.New("unterminated quote")
	}
	if inArg {
		args = append(args, current.String())
	}
	return args, nil
}

func buildCompleter(rootCmd *cobra.Command) *readline.PrefixCompleter {
	var items []readline.PrefixCompleterInterface

	for _, cmd := range rootCmd.Commands() {
		if cmd.Hidden || cmd.Name() == "shell" {
			continue
		}

		var subItems []readline.PrefixCompleterInterface
		for _, subCmd := range cmd.Commands() {
			if !subCmd.Hidden {
				subItems = append(subItems, readline.PcItem(subCmd.Name()))
			}
		}

		if len(subItems) > 0 {
			items = append(items, readline.PcItem(cmd.Name(), subItems...))
		} else {
			items = append(items, readline.PcItem(cmd.Name()))
		}
	}

	items = append(items,
		readline.PcItem("exit"),
		readline.PcItem("quit"),
		readline.PcItem("help"),
	)

	return readline.NewPrefixCompleter(items...)
}
