package config

import (
	"os"
	"strings"
	"testing"
)

func TestResolvePreferTcp(t *testing.T) {
	tests := []struct {
		name       string
		env        string
		stored     bool
		want       bool
		wantSource Source
		wantErr    bool
	}{
		{"nothing set", "", false, false, SourceDefault, false},
		{"saved", "", true, true, SourceConfig, false},
		{"env on", "true", false, true, SourceEnv, false},
		{"env 1", "1", false, true, SourceEnv, false},
		{"env off overrides saved", "false", true, false, SourceEnv, false},
		{"invalid env falls back to saved", "yes please", true, true, SourceConfig, true},
		{"invalid env falls back to default", "on", false, false, SourceDefault, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(PreferTcpEnv, tt.env)

			got, source, err := ResolvePreferTcp(Settings{PreferTcp: tt.stored})
			if got != tt.want || source != tt.wantSource || (err != nil) != tt.wantErr {
				t.Errorf("ResolvePreferTcp() = (%v, %q, %v), want (%v, %q, error %v)", got, source, err, tt.want, tt.wantSource, tt.wantErr)
			}
		})
	}
}

// useTestConfigFile points GetConfigManager at a temp file holding content.
func useTestConfigFile(t *testing.T, content string) {
	t.Helper()
	GetConfigManager()
	orig := instance
	cm := newTestConfigManager(t, &Config{})
	if err := os.WriteFile(cm.filePath, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	instance = cm
	t.Cleanup(func() { instance = orig })
}

func TestPreferTcpReadsSavedConfig(t *testing.T) {
	t.Setenv(PreferTcpEnv, "")
	useTestConfigFile(t, `{"preferTcp":true}`)

	got, source, err := PreferTcp()
	if err != nil || !got || source != SourceConfig {
		t.Errorf("PreferTcp() = (%v, %q, %v), want (true, %q, nil)", got, source, err, SourceConfig)
	}
}

func TestSettingPreferTcp(t *testing.T) {
	t.Setenv(PreferTcpEnv, "")
	s, err := LookupSetting("prefer-tcp")
	if err != nil {
		t.Fatalf("LookupSetting: %v", err)
	}

	var settings Settings
	if saved, err := s.Set(&settings, "1"); err != nil || saved != "true" {
		t.Fatalf("Set(1) = (%q, %v), want \"true\"", saved, err)
	}
	if !settings.PreferTcp {
		t.Error("Set(1) didn't turn PreferTcp on")
	}
	if value, source, _ := s.Resolve(settings); value != "true" || source != SourceConfig {
		t.Errorf("Resolve() = (%q, %q), want (\"true\", %q)", value, source, SourceConfig)
	}

	if _, err := s.Set(&settings, "maybe"); err == nil {
		t.Error("Set(maybe) succeeded, want an error")
	}
	if !settings.PreferTcp {
		t.Error("an invalid Set changed the setting")
	}

	if saved, err := s.Unset(&settings); err != nil || saved != "false" {
		t.Fatalf("Unset() = (%q, %v), want \"false\"", saved, err)
	}
	if value, source, _ := s.Resolve(settings); value != "false" || source != SourceDefault {
		t.Errorf("Resolve() after Unset = (%q, %q), want (\"false\", %q)", value, source, SourceDefault)
	}
}

func TestSettingListenAddressIsReadOnly(t *testing.T) {
	s, err := LookupSetting("listen-address")
	if err != nil {
		t.Fatalf("LookupSetting: %v", err)
	}
	if s.Settable() {
		t.Fatal("listen-address is settable, want read-only")
	}

	settings := Settings{ApiListenAddr: "0.0.0.0:4000"}
	if _, err := s.Set(&settings, "0.0.0.0:5000"); err == nil || !strings.Contains(err.Error(), "--listen-address") {
		t.Errorf("Set error = %v, want a hint about --listen-address", err)
	}
	if _, err := s.Unset(&settings); err == nil {
		t.Error("Unset succeeded, want an error")
	}
	if settings.ApiListenAddr != "0.0.0.0:4000" {
		t.Errorf("ApiListenAddr = %q, want it unchanged", settings.ApiListenAddr)
	}

	if value, source, _ := s.Resolve(settings); value != "0.0.0.0:4000" || source != SourceConfig {
		t.Errorf("Resolve() = (%q, %q), want saved address from config", value, source)
	}
	if value, source, _ := s.Resolve(Settings{}); value != DefaultAPIAddress || source != SourceDefault {
		t.Errorf("Resolve() with nothing saved = (%q, %q), want default address", value, source)
	}
}

func TestLookupSettingUnknownKey(t *testing.T) {
	_, err := LookupSetting("accountId")
	if err == nil {
		t.Fatal("LookupSetting(accountId) succeeded, want an error")
	}
	for _, key := range []string{"listen-address", "prefer-tcp"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q doesn't list known key %q", err, key)
		}
	}
}
