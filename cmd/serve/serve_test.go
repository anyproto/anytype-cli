package serve

import (
	"strings"
	"testing"

	"github.com/anyproto/anytype-cli/core/config"
)

func TestNewServeCmd(t *testing.T) {
	cmd := NewServeCmd()

	if cmd.Use != "serve" {
		t.Errorf("cmd.Use = %v, want serve", cmd.Use)
	}

	if len(cmd.Aliases) != 1 || cmd.Aliases[0] != "start" {
		t.Errorf("cmd.Aliases = %v, want [start]", cmd.Aliases)
	}
}

func TestServeCmd_ListenAddressFlag(t *testing.T) {
	cmd := NewServeCmd()

	flag := cmd.Flag("listen-address")
	if flag == nil {
		t.Fatal("listen-address flag not found")
		return
	}

	if flag.DefValue != config.DefaultAPIAddress {
		t.Errorf("listen-address default = %v, want %v", flag.DefValue, config.DefaultAPIAddress)
	}

	if flag.Usage != "JSON API listen address in `host:port` format (remembered for later commands)" {
		t.Errorf("listen-address usage = %v, want %q", flag.Usage, "JSON API listen address in `host:port` format (remembered for later commands)")
	}
}

func TestServeCmd_ListenAddressFlagCustomValue(t *testing.T) {
	cmd := NewServeCmd()

	customAddr := "0.0.0.0:8080"
	cmd.SetArgs([]string{"--listen-address", customAddr})

	if err := cmd.ParseFlags([]string{"--listen-address", customAddr}); err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

	flag := cmd.Flag("listen-address")
	if flag.Value.String() != customAddr {
		t.Errorf("listen-address value = %v, want %v", flag.Value.String(), customAddr)
	}
}

func TestServeCmd_QuietFlag(t *testing.T) {
	cmd := NewServeCmd()

	flag := cmd.Flag("quiet")
	if flag == nil {
		t.Fatal("quiet flag not found")
	}

	if flag.Shorthand != "q" {
		t.Errorf("quiet shorthand = %v, want q", flag.Shorthand)
	}

	if flag.DefValue != "false" {
		t.Errorf("quiet default = %v, want false", flag.DefValue)
	}
}

func TestServeCmd_VerboseFlag(t *testing.T) {
	cmd := NewServeCmd()

	flag := cmd.Flag("verbose")
	if flag == nil {
		t.Fatal("verbose flag not found")
	}

	if flag.Shorthand != "v" {
		t.Errorf("verbose shorthand = %v, want v", flag.Shorthand)
	}

	if flag.DefValue != "false" {
		t.Errorf("verbose default = %v, want false", flag.DefValue)
	}
}

func TestServeCmd_QuietAndVerboseMutuallyExclusive(t *testing.T) {
	cmd := NewServeCmd()
	cmd.SetArgs([]string{"--quiet", "--verbose"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when using --quiet and --verbose together")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "quiet") {
		t.Errorf("error message should mention 'quiet' flag, got: %q", errMsg)
	}
	if !strings.Contains(errMsg, "verbose") {
		t.Errorf("error message should mention 'verbose' flag, got: %q", errMsg)
	}
}
