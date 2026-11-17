package localstate

import (
	"path/filepath"
	"testing"
)

func TestLockAndAtomicPrivateFiles(t *testing.T) {
	dir := t.TempDir()
	unlock, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Lock(dir); err != ErrBusy {
		t.Fatal(err)
	}
	unlock()
	unlock2, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	unlock2()
	file := filepath.Join(dir, "state.json")
	if err = WriteJSON(file, map[string]int{"version": 1}); err != nil {
		t.Fatal(err)
	}
	if err = WriteJSON(file, map[string]int{"version": 2}); err != nil {
		t.Fatal(err)
	}
	var v map[string]int
	if err = ReadJSON(file, &v); err != nil || v["version"] != 2 {
		t.Fatal(v, err)
	}
	if err = ReadPrivateJSON(file, &v); err != nil || v["version"] != 2 {
		t.Fatal("private file roundtrip", v, err)
	}
}

func TestHumanMutationConfig(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"config", "list"}, false},
		{[]string{"config", "get", "prefer-tcp"}, false},
		{[]string{"config", "set", "prefer-tcp", "true"}, true},
		{[]string{"config", "unset", "prefer-tcp"}, true},
		{[]string{"config", "set", "--help"}, false},
	}
	for _, tt := range tests {
		if got := HumanMutation(tt.args); got != tt.want {
			t.Errorf("HumanMutation(%q) = %v, want %v", tt.args, got, tt.want)
		}
	}
}
