package config

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coreconfig "github.com/anyproto/anytype-cli/core/config"
)

// TestMain points HOME at a temp dir before the config manager first reads
// it, so the commands work on a throwaway config file.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "anytype-config-cmd")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

func configPath(t *testing.T) string {
	t.Helper()
	return coreconfig.GetConfigManager().GetFilePath()
}

// writeConfig replaces the config file; empty content removes it.
func writeConfig(t *testing.T, content string) {
	t.Helper()
	path := configPath(t)
	if content == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func readConfig(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(configPath(t))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

// runConfig runs `anytype config args...` and returns what it printed.
func runConfig(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	capture := func(target **os.File) func() string {
		r, w, pipeErr := os.Pipe()
		if pipeErr != nil {
			t.Fatal(pipeErr)
		}
		orig := *target
		*target = w
		var buf bytes.Buffer
		done := make(chan struct{})
		go func() {
			_, _ = io.Copy(&buf, r)
			close(done)
		}()
		return func() string {
			*target = orig
			w.Close()
			<-done
			return buf.String()
		}
	}
	stopOut := capture(&os.Stdout)
	stopErr := capture(&os.Stderr)

	cmd := NewConfigCmd()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	err = cmd.Execute()

	return stopOut(), stopErr(), err
}

func TestConfigListDefaults(t *testing.T) {
	t.Setenv(coreconfig.PreferTcpEnv, "")
	writeConfig(t, "")

	stdout, _, err := runConfig(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{
		"listen-address   " + coreconfig.DefaultAPIAddress,
		"prefer-tcp       false",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("list output missing %q:\n%s", want, stdout)
		}
	}
	if strings.Count(stdout, "default") != 2 {
		t.Errorf("want both keys from default:\n%s", stdout)
	}
}

func TestConfigSetGetUnsetPreferTcp(t *testing.T) {
	t.Setenv(coreconfig.PreferTcpEnv, "")
	writeConfig(t, `{"accountId":"acc","sessionToken":"token"}`)

	stdout, _, err := runConfig(t, "set", "prefer-tcp", "1")
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if !strings.Contains(stdout, "prefer-tcp = true") || !strings.Contains(stdout, "anytype service restart") {
		t.Errorf("set output = %q, want the saved value and a restart hint", stdout)
	}
	raw := readConfig(t)
	if raw["preferTcp"] != true || raw["accountId"] != "acc" || raw["sessionToken"] != "token" {
		t.Errorf("config after set = %v, want preferTcp added and account fields kept", raw)
	}

	stdout, _, err = runConfig(t, "get", "prefer-tcp")
	if err != nil || stdout != "true\n" {
		t.Errorf("get = (%q, %v), want \"true\\n\"", stdout, err)
	}

	stdout, _, err = runConfig(t, "list")
	if err != nil || !strings.Contains(stdout, "true                     config") {
		t.Errorf("list = (%q, %v), want prefer-tcp true from config", stdout, err)
	}

	stdout, _, err = runConfig(t, "unset", "prefer-tcp")
	if err != nil || !strings.Contains(stdout, "prefer-tcp = false (default)") {
		t.Errorf("unset = (%q, %v)", stdout, err)
	}
	raw = readConfig(t)
	if _, ok := raw["preferTcp"]; ok || raw["accountId"] != "acc" {
		t.Errorf("config after unset = %v, want preferTcp removed and account fields kept", raw)
	}
}

func TestConfigSetRejectsInvalidValue(t *testing.T) {
	writeConfig(t, `{"preferTcp":true}`)

	if _, _, err := runConfig(t, "set", "prefer-tcp", "maybe"); err == nil {
		t.Fatal("set prefer-tcp maybe succeeded, want an error")
	}
	if raw := readConfig(t); raw["preferTcp"] != true {
		t.Errorf("config after a rejected set = %v, want it unchanged", raw)
	}
}

func TestConfigListenAddressIsReadOnly(t *testing.T) {
	writeConfig(t, `{"apiListenAddr":"0.0.0.0:4000"}`)

	for _, args := range [][]string{{"set", "listen-address", "0.0.0.0:5000"}, {"unset", "listen-address"}} {
		_, _, err := runConfig(t, args...)
		if err == nil || !strings.Contains(err.Error(), "--listen-address") {
			t.Errorf("%v error = %v, want a hint about --listen-address", args, err)
		}
	}
	if raw := readConfig(t); raw["apiListenAddr"] != "0.0.0.0:4000" {
		t.Errorf("config = %v, want apiListenAddr unchanged", raw)
	}

	stdout, _, err := runConfig(t, "get", "listen-address")
	if err != nil || stdout != "0.0.0.0:4000\n" {
		t.Errorf("get listen-address = (%q, %v)", stdout, err)
	}
}

func TestConfigEnvOverride(t *testing.T) {
	t.Setenv(coreconfig.PreferTcpEnv, "true")
	writeConfig(t, "")

	stdout, _, err := runConfig(t, "get", "prefer-tcp")
	if err != nil || stdout != "true\n" {
		t.Errorf("get = (%q, %v), want the environment value", stdout, err)
	}

	stdout, _, err = runConfig(t, "list")
	if err != nil || !strings.Contains(stdout, "env "+coreconfig.PreferTcpEnv) {
		t.Errorf("list = (%q, %v), want the environment as the source", stdout, err)
	}

	_, stderr, err := runConfig(t, "unset", "prefer-tcp")
	if err != nil || !strings.Contains(stderr, coreconfig.PreferTcpEnv+"=true") {
		t.Errorf("unset = (stderr %q, %v), want a warning about the override", stderr, err)
	}
}

func TestConfigInvalidEnvWarns(t *testing.T) {
	t.Setenv(coreconfig.PreferTcpEnv, "maybe")
	writeConfig(t, `{"preferTcp":true}`)

	stdout, stderr, err := runConfig(t, "get", "prefer-tcp")
	if err != nil || stdout != "true\n" || !strings.Contains(stderr, "ignoring "+coreconfig.PreferTcpEnv) {
		t.Errorf("get = (%q, stderr %q, %v), want the saved value and a warning", stdout, stderr, err)
	}
}

func TestConfigUnknownKey(t *testing.T) {
	for _, args := range [][]string{{"get", "accountId"}, {"set", "accountId", "x"}, {"unset", "accountId"}} {
		if _, _, err := runConfig(t, args...); err == nil || !strings.Contains(err.Error(), "unknown config key") {
			t.Errorf("%v error = %v, want unknown config key", args, err)
		}
	}
}
