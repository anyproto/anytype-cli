package config

import (
	"os"
	"path/filepath"
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
			NetworkId:   "test-network-456",
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
		if cfg.NetworkId != testConfig.NetworkId {
			t.Errorf("NetworkId = %v, want %v", cfg.NetworkId, testConfig.NetworkId)
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
