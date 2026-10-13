package cmdutil

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/output"
)

// SettingKeyCompletion completes the first argument with config keys.
func SettingKeyCompletion(settableOnly bool) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var keys []cobra.Completion
		for _, s := range config.SettingKeys() {
			if !settableOnly || s.Settable() {
				keys = append(keys, s.Key)
			}
		}
		return keys, cobra.ShellCompDirectiveNoFileComp
	}
}

// SourceLabel names where a setting's value comes from, including the
// environment variable when it's an override.
func SourceLabel(source config.Source, env string) string {
	if source == config.SourceEnv {
		return "env " + env
	}
	return string(source)
}

// ExplainSettingChange tells the user when a saved change takes effect.
func ExplainSettingChange(s config.Setting) {
	if s.Env != "" {
		if value := os.Getenv(s.Env); value != "" {
			output.Warning("%s=%s is set here and overrides the saved value for commands run from this environment.", s.Env, value)
		}
	}
	if s.RestartToApply {
		output.Print("Applies the next time the account starts. To apply it now, restart the server:")
		output.Print("  anytype service restart   (or stop and start 'anytype serve')")
	}
}
