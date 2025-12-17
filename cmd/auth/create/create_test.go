package create

import (
	"testing"
)

func TestCreateCommand(t *testing.T) {
	cmd := NewCreateCmd()

	if cmd.Use != "create <name>" {
		t.Errorf("Use = %v, want create <name>", cmd.Use)
	}

	if cmd.Flag("root-path") == nil {
		t.Error("root-path flag not found")
	}
	if cmd.Flag("listen-address") == nil {
		t.Error("listen-address flag not found")
	}
}

func TestCreateCommandRequiresName(t *testing.T) {
	cmd := NewCreateCmd()

	if cmd.Args == nil {
		t.Error("Args validator is not set")
	}
	if cmd.RunE == nil {
		t.Error("RunE function is not set")
	}
}
