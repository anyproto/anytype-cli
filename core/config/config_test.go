package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestConfigManager(t *testing.T) {
	t.Run("SaveAndLoad", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "anytype-config-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		configPath := filepath.Join(tempDir, "config.json")
		testConfig := &Config{
			AccountId:   "test-account-123",
			TechSpaceId: "test-tech-space-789",
		}

		cm := &ConfigManager{
			config:   testConfig,
			filePath: configPath,
		}

		err = cm.Save()
		if err != nil {
			t.Errorf("Save failed: %v", err)
		}

		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Error("Config file was not created")
		}

		cm2 := &ConfigManager{
			config:   &Config{},
			filePath: configPath,
		}

		err = cm2.Load()
		if err != nil {
			t.Errorf("Load failed: %v", err)
		}

		cfg := cm2.Get()
		if cfg.AccountId != testConfig.AccountId {
			t.Errorf("AccountId = %v, want %v", cfg.AccountId, testConfig.AccountId)
		}
		if cfg.TechSpaceId != testConfig.TechSpaceId {
			t.Errorf("TechSpaceId = %v, want %v", cfg.TechSpaceId, testConfig.TechSpaceId)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "anytype-config-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		configPath := filepath.Join(tempDir, "config.json")
		cm := &ConfigManager{
			config:   &Config{AccountId: "test"},
			filePath: configPath,
		}

		_ = cm.Save()

		err = cm.Delete()
		if err != nil {
			t.Errorf("Delete failed: %v", err)
		}

		if _, err := os.Stat(configPath); !os.IsNotExist(err) {
			t.Error("Config file still exists after delete")
		}
	})
}

func TestGetConfigManager(t *testing.T) {
	cm := GetConfigManager()
	if cm == nil {
		t.Fatal("GetConfigManager returned nil")
	}

	cm2 := GetConfigManager()
	if cm != cm2 {
		t.Error("GetConfigManager did not return singleton instance")
	}

	if cm.filePath == "" {
		t.Error("GetConfigManager should initialize with a valid file path")
	}
}

func newTestConfigManager(t *testing.T, cfg *Config) *ConfigManager {
	t.Helper()
	return &ConfigManager{config: cfg, filePath: filepath.Join(t.TempDir(), "config.json")}
}

func TestApiListenAddrPersists(t *testing.T) {
	cm := newTestConfigManager(t, &Config{})

	if err := cm.SetApiListenAddr("0.0.0.0:4000"); err != nil {
		t.Fatalf("SetApiListenAddr: %v", err)
	}

	cm2 := &ConfigManager{config: &Config{}, filePath: cm.filePath}
	if err := cm2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cm2.Get().ApiListenAddr; got != "0.0.0.0:4000" {
		t.Errorf("ApiListenAddr = %q, want %q", got, "0.0.0.0:4000")
	}
}

func TestClearAccountKeepsSettings(t *testing.T) {
	settings := Settings{ApiListenAddr: "0.0.0.0:4000", PreferTcp: true}
	cm := newTestConfigManager(t, &Config{
		AccountId:    "acc",
		TechSpaceId:  "tech",
		AccountKey:   "key",
		SessionToken: "token",
		Settings:     settings,
	})
	if err := cm.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := cm.ClearAccount(); err != nil {
		t.Fatalf("ClearAccount: %v", err)
	}

	cm2 := &ConfigManager{config: &Config{}, filePath: cm.filePath}
	if err := cm2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{Settings: settings}
	if got := *cm2.Get(); got != want {
		t.Errorf("config after ClearAccount = %+v, want %+v", got, want)
	}
}

func TestSettingsStayAtTopLevelOfFile(t *testing.T) {
	cm := newTestConfigManager(t, &Config{
		AccountId: "acc",
		Settings:  Settings{ApiListenAddr: "0.0.0.0:4000", PreferTcp: true},
	})
	if err := cm.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(cm.filePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	want := map[string]any{"accountId": "acc", "apiListenAddr": "0.0.0.0:4000", "preferTcp": true}
	if !reflect.DeepEqual(raw, want) {
		t.Errorf("config file = %v, want %v", raw, want)
	}
}

func TestUpdateSettings(t *testing.T) {
	cm := newTestConfigManager(t, &Config{AccountId: "acc"})

	if err := cm.UpdateSettings(func(s *Settings) error {
		s.PreferTcp = true
		return nil
	}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	cm2 := &ConfigManager{config: &Config{}, filePath: cm.filePath}
	if err := cm2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := *cm2.Get(); got != (Config{AccountId: "acc", Settings: Settings{PreferTcp: true}}) {
		t.Errorf("config after UpdateSettings = %+v", got)
	}

	failed := errors.New("rejected")
	err := cm.UpdateSettings(func(s *Settings) error {
		s.PreferTcp = false
		return failed
	})
	if !errors.Is(err, failed) {
		t.Fatalf("UpdateSettings error = %v, want %v", err, failed)
	}
	if !cm.Get().PreferTcp {
		t.Error("UpdateSettings changed the settings although fn failed")
	}
}

func TestLoadDropsKeysRemovedFromFile(t *testing.T) {
	cm := newTestConfigManager(t, &Config{AccountId: "acc", Settings: Settings{PreferTcp: true}})
	if err := os.WriteFile(cm.filePath, []byte(`{"accountId":"acc"}`), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := cm.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cm.Get().PreferTcp {
		t.Error("PreferTcp survived Load although the file no longer has it")
	}

	if err := os.Remove(cm.filePath); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := cm.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := *cm.Get(); got != (Config{}) {
		t.Errorf("config after loading a missing file = %+v, want empty", got)
	}
}

func TestClearAccountRemovesFileWithoutSettings(t *testing.T) {
	cm := newTestConfigManager(t, &Config{AccountId: "acc", SessionToken: "token"})
	if err := cm.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := cm.ClearAccount(); err != nil {
		t.Fatalf("ClearAccount: %v", err)
	}

	if _, err := os.Stat(cm.filePath); !os.IsNotExist(err) {
		t.Errorf("config file still exists after clearing an account with no settings to keep (err = %v)", err)
	}
}
