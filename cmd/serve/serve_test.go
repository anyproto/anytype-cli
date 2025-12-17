package serve

import (
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

	if flag.Usage != "API listen address in `host:port` format" {
		t.Errorf("listen-address usage = %v, want 'API listen address in `host:port` format'", flag.Usage)
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
