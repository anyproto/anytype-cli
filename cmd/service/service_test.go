package service

import (
	"testing"
)

func TestNewServiceCmd(t *testing.T) {
	cmd := NewServiceCmd()

	if cmd.Use != "service <command>" {
		t.Errorf("cmd.Use = %v, want 'service <command>'", cmd.Use)
	}

	if cmd.Short != "Manage anytype as a user service" {
		t.Errorf("cmd.Short = %v, want 'Manage anytype as a user service'", cmd.Short)
	}
}

func TestServiceCmd_HasAllSubcommands(t *testing.T) {
	cmd := NewServiceCmd()

	expectedSubcommands := []string{
		"install",
		"uninstall",
		"start",
		"stop",
		"restart",
		"status",
	}

	subcommands := cmd.Commands()
	if len(subcommands) != len(expectedSubcommands) {
		t.Errorf("service has %d subcommands, want %d", len(subcommands), len(expectedSubcommands))
	}

	subcommandMap := make(map[string]bool)
	for _, sub := range subcommands {
		subcommandMap[sub.Use] = true
	}

	for _, expected := range expectedSubcommands {
		if !subcommandMap[expected] {
			t.Errorf("subcommand %q not found", expected)
		}
	}
}

func TestServiceCmd_InstallHasListenAddressFlag(t *testing.T) {
	cmd := NewServiceCmd()

	installCmd, _, err := cmd.Find([]string{"install"})
	if err != nil {
		t.Fatalf("Failed to find install subcommand: %v", err)
	}

	flag := installCmd.Flag("listen-address")
	if flag == nil {
		t.Fatal("install subcommand should have listen-address flag")
	}
}
