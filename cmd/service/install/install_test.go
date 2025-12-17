package install

import (
	"testing"

	"github.com/anyproto/anytype-cli/core/config"
)

func TestNewInstallCmd(t *testing.T) {
	cmd := NewInstallCmd()

	if cmd.Use != "install" {
		t.Errorf("cmd.Use = %v, want install", cmd.Use)
	}

	if cmd.Short != "Install as a user service" {
		t.Errorf("cmd.Short = %v, want 'Install as a user service'", cmd.Short)
	}
}

func TestInstallCmd_ListenAddressFlag(t *testing.T) {
	cmd := NewInstallCmd()

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

func TestInstallCmd_ListenAddressFlagCustomValue(t *testing.T) {
	cmd := NewInstallCmd()

	customAddr := "0.0.0.0:9000"

	if err := cmd.ParseFlags([]string{"--listen-address", customAddr}); err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

	flag := cmd.Flag("listen-address")
	if flag.Value.String() != customAddr {
		t.Errorf("listen-address value = %v, want %v", flag.Value.String(), customAddr)
	}
}
