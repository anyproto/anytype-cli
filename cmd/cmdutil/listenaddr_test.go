package cmdutil

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/core/config"
)

func TestAPIListenAddr(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		stored  string
		want    string
		changed bool
	}{
		{"flag given", []string{"--listen-address", "0.0.0.0:5000"}, "127.0.0.1:4000", "0.0.0.0:5000", true},
		{"flag omitted uses stored", nil, "127.0.0.1:4000", "127.0.0.1:4000", false},
		{"flag omitted, nothing stored", nil, "", config.DefaultAPIAddress, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := storedAPIListenAddr
			storedAPIListenAddr = func() (string, error) { return tt.stored, nil }
			t.Cleanup(func() { storedAPIListenAddr = orig })

			var flag string
			cmd := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
			AddListenAddressFlag(cmd, &flag)
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}

			got, changed := APIListenAddr(cmd, flag)
			if got != tt.want || changed != tt.changed {
				t.Errorf("APIListenAddr() = (%q, %v), want (%q, %v)", got, changed, tt.want, tt.changed)
			}
		})
	}
}
