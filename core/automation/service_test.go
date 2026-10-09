package automation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/localstate"
)

func TestNativeServiceInspectionDoesNotCreateDirectories(t *testing.T) {
	dir := t.TempDir()
	called := false
	n := &NativeService{Platform: "darwin", Home: dir, Run: func(context.Context, string, ...string) ([]byte, error) { called = true; return nil, nil }}
	s, err := n.Status(context.Background())
	if err != nil || s.Installed || called {
		t.Fatal(s, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("read created directories")
	}
}
func TestLaunchdBindingAndSupervisorPID(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "anytype")
	if err := os.WriteFile(exe, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	definition := filepath.Join(dir, "Library/LaunchAgents/anytype.plist")
	if err := os.MkdirAll(filepath.Dir(definition), 0700); err != nil {
		t.Fatal(err)
	}
	xml := `<plist><dict><key>ProgramArguments</key><array><string>` + exe + `</string><string>serve</string><string>--listen-address</string><string>127.0.0.1:4000</string></array></dict></plist>`
	if err := os.WriteFile(definition, []byte(xml), 0600); err != nil {
		t.Fatal(err)
	}
	n := &NativeService{Platform: "darwin", Home: dir, Executable: exe, Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "launchctl" || strings.Join(args, " ") != "list anytype" {
			t.Fatal(name, args)
		}
		return []byte(`{ "PID" = 123;`), nil
	}}
	s, err := n.Status(context.Background())
	if err != nil || !s.Installed || !s.BindingVerified || s.ListenAddress != "127.0.0.1:4000" || s.PID != 123 || s.DefinitionHash == "" {
		t.Fatal(s, err)
	}
}
func TestUnitParserDoesNotEvaluateShellOrEnvironment(t *testing.T) {
	if got := parseUnitArgs(`"/opt/Anytype CLI/anytype" "serve" "--listen-address" "127.0.0.1:4000"`); len(got) != 4 || got[0] != "/opt/Anytype CLI/anytype" {
		t.Fatal(got)
	}
	for _, value := range []string{`/bin/anytype $ARGS`, `/bin/anytype %h/serve`, `"unterminated`} {
		if got := parseUnitArgs(value); got != nil {
			t.Fatal(got)
		}
	}
}

func TestServiceDefinitionInheritsSavedListenAddress(t *testing.T) {
	for _, platform := range []string{"darwin", "linux"} {
		for _, explicit := range []bool{false, true} {
			dir := t.TempDir()
			exe := filepath.Join(dir, "anytype")
			if err := os.WriteFile(exe, []byte("fixture"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := localstate.WriteJSON(filepath.Join(dir, config.AnytypeDirName, config.ConfigFileName), config.Config{ApiListenAddr: "127.0.0.1:4500"}); err != nil {
				t.Fatal(err)
			}
			definition := filepath.Join(dir, "Library/LaunchAgents/anytype.plist")
			content := `<plist><dict><key>ProgramArguments</key><array><string>` + exe + `</string><string>serve</string>`
			if explicit {
				content += `<string>--listen-address</string><string>127.0.0.1:4600</string>`
			}
			content += `</array></dict></plist>`
			if platform == "linux" {
				definition = filepath.Join(dir, ".config/systemd/user/anytype.service")
				content = "[Service]\nExecStart=" + exe + " serve"
				if explicit {
					content += " --listen-address 127.0.0.1:4600"
				}
			}
			if err := os.MkdirAll(filepath.Dir(definition), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(definition, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			n := &NativeService{Platform: platform, Home: dir, Executable: exe, Run: func(context.Context, string, ...string) ([]byte, error) {
				args := exe + " serve"
				if explicit {
					args += " --listen-address 127.0.0.1:4600"
				}
				return []byte("ActiveState=inactive\nFragmentPath=" + definition + "\nLoadState=loaded\nNeedDaemonReload=no\nExecStart={ path=" + exe + " ; argv[]=" + args + " ; ignore_errors=no ; start_time=[n/a] ; stop_time=[n/a] ; pid=0 ; code=(null) ; status=0 }"), nil
			}}
			s, err := n.Status(context.Background())
			want := "127.0.0.1:4500"
			if explicit {
				want = "127.0.0.1:4600"
			}
			if err != nil || !s.BindingVerified || s.ListenAddress != want {
				t.Fatal(platform, explicit, s, err)
			}
		}
	}
}

func TestSystemdLoadedBindingMustMatchCurrentDefinition(t *testing.T) {
	for _, scenario := range []string{"matching", "stale", "wrong_executable", "wrong_listener", "missing_reload_state", "unloaded", "multiple_commands", "unparseable"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			exe := filepath.Join(dir, "anytype")
			if err := os.WriteFile(exe, []byte("fixture"), 0700); err != nil {
				t.Fatal(err)
			}
			definition := filepath.Join(dir, ".config/systemd/user/anytype.service")
			if err := os.MkdirAll(filepath.Dir(definition), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(definition, []byte("[Service]\nExecStart="+exe+" serve --listen-address 127.0.0.1:4500\n"), 0600); err != nil {
				t.Fatal(err)
			}
			loadedExe, listener, reload, loadState := exe, "127.0.0.1:4500", "no", "loaded"
			switch scenario {
			case "stale":
				reload = "yes"
			case "wrong_executable":
				loadedExe = "/old/anytype"
			case "wrong_listener":
				listener = "127.0.0.1:4600"
			case "missing_reload_state":
				reload = ""
			case "unloaded":
				loadState = "not-found"
			}
			command := "{ path=" + loadedExe + " ; argv[]=" + loadedExe + " serve --listen-address " + listener + " ; ignore_errors=no ; start_time=[n/a] ; stop_time=[n/a] ; pid=0 ; code=(null) ; status=0 }"
			if scenario == "multiple_commands" {
				command += " " + command
			}
			if scenario == "unparseable" {
				command = "unknown"
			}
			calls := 0
			n := &NativeService{Platform: "linux", Home: dir, Executable: exe, Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
				calls++
				if name != "systemctl" || len(args) != 4 || args[1] != "show" || !strings.Contains(args[3], "NeedDaemonReload,ExecStart") {
					t.Fatal("unexpected supervisor operation", name, args)
				}
				return []byte("ActiveState=inactive\nMainPID=0\nFragmentPath=" + definition + "\nLoadState=" + loadState + "\nNeedDaemonReload=" + reload + "\nExecStart=" + command), nil
			}}
			s, err := n.Status(context.Background())
			if err != nil || s.BindingVerified != (scenario == "matching") || calls != 1 {
				t.Fatal(s, err, calls)
			}
			if scenario != "matching" {
				e, _, _ := fixture(t, false)
				e.Service = n
				for _, command := range []string{"service install", "service start"} {
					assertCode(t, run(e, Request{Command: command, OperationId: "service-loaded", ListenAddress: "127.0.0.1:4500"}), "service_binding_conflict")
				}
			}
		})
	}
}

func TestLaunchdProgramOverrideIsNeverVerified(t *testing.T) {
	for _, before := range []bool{true, false} {
		dir := t.TempDir()
		exe := filepath.Join(dir, "anytype")
		if err := os.WriteFile(exe, []byte("fixture"), 0700); err != nil {
			t.Fatal(err)
		}
		arguments := `<key>ProgramArguments</key><array><string>` + exe + `</string><string>serve</string></array>`
		override := `<key>Program</key><string>/some/other/executable</string>`
		content := arguments + override
		if before {
			content = override + arguments
		}
		definition := filepath.Join(dir, "Library/LaunchAgents/anytype.plist")
		if err := os.MkdirAll(filepath.Dir(definition), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(definition, []byte(`<plist><dict>`+content+`</dict></plist>`), 0600); err != nil {
			t.Fatal(err)
		}
		n := &NativeService{Platform: "darwin", Home: dir, Executable: exe, Run: func(context.Context, string, ...string) ([]byte, error) { return nil, nil }}
		s, err := n.Status(context.Background())
		if err != nil || !s.Installed || s.BindingVerified {
			t.Fatal(s, err)
		}
	}
}
