package shell

import (
	"errors"
	"io"
	"strings"

	"github.com/chzyer/readline"
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/core/output"
)

func NewShellCmd(rootCmd *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "shell",
		Short: "Start interactive shell mode",
		Long:  "Launch an interactive shell where you can run Anytype commands without the 'anytype' prefix. Type 'exit' to quit.",
		RunE: func(cmd *cobra.Command, args []string) error {
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

		args := strings.Split(line, " ")
		if args[0] == "shell" {
			output.Warning("Already in shell mode. Type 'exit' or 'quit' to leave.")
			continue
		}
		rootCmd.SetArgs(args)

		if err := rootCmd.Execute(); err != nil {
			output.Warning("Command error: %v", err)
		}
	}
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
