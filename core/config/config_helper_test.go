package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// setupTestHome creates a temp directory and sets HOME to it for test isolation
func setupTestHome(t *testing.T) string {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "anytype-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })
	t.Setenv("HOME", tempDir)

	// Reset the singleton so GetConfigManager uses the test HOME-based path
	instance = nil
	once = sync.Once{}

	return tempDir
}

func TestGetStoredAccountId(t *testing.T) {
	tempDir := setupTestHome(t)

	configPath := filepath.Join(tempDir, ".anytype", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}
	testConfig := `{"accountId":"test-account-123"}`
	if err := os.WriteFile(configPath, []byte(testConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	accountId, err := GetAccountIdFromConfig()
	if err != nil {
		t.Fatalf("GetAccountIdFromConfig() returned error: %v", err)
	}
	if accountId != "test-account-123" {
		t.Fatalf("GetAccountIdFromConfig() = %v, want test-account-123", accountId)
	}
}

func TestGetStoredTechSpaceId(t *testing.T) {
	tempDir := setupTestHome(t)

	configPath := filepath.Join(tempDir, ".anytype", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}
	testConfig := `{"techSpaceId":"tech-space-789"}`
	if err := os.WriteFile(configPath, []byte(testConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	techSpaceId, err := GetTechSpaceIdFromConfig()
	if err != nil {
		t.Fatalf("GetTechSpaceIdFromConfig() returned error: %v", err)
	}
	if techSpaceId != "tech-space-789" {
		t.Fatalf("GetTechSpaceIdFromConfig() = %v, want tech-space-789", techSpaceId)
	}
}

func TestLoadStoredConfig(t *testing.T) {
	tempDir := setupTestHome(t)

	configPath := filepath.Join(tempDir, ".anytype", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}
	testConfig := `{
		"accountId":"test-account-123",
		"techSpaceId":"tech-space-789"
	}`
	if err := os.WriteFile(configPath, []byte(testConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadStoredConfig()
	if err != nil {
		t.Fatalf("LoadStoredConfig() returned error: %v", err)
	}
	if cfg == nil {
		t.Fatalf("LoadStoredConfig() returned nil config")
	}
	if cfg.AccountId != "test-account-123" {
		t.Fatalf("LoadStoredConfig() AccountId = %v, want test-account-123", cfg.AccountId)
	}
	if cfg.TechSpaceId != "tech-space-789" {
		t.Fatalf("LoadStoredConfig() TechSpaceId = %v, want tech-space-789", cfg.TechSpaceId)
	}
}

func TestNetworkIdHelpers(t *testing.T) {
	tempDir := setupTestHome(t)

	configPath := filepath.Join(tempDir, ".anytype", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	if err := os.WriteFile(configPath, []byte(`{"networkId":"net-123"}`), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	networkId, err := GetNetworkIdFromConfig()
	if err != nil {
		t.Fatalf("GetNetworkIdFromConfig() returned error: %v", err)
	}
	if networkId != "net-123" {
		t.Fatalf("GetNetworkIdFromConfig() = %v, want net-123", networkId)
	}

	if err := SetNetworkIdToConfig("net-456"); err != nil {
		t.Fatalf("SetNetworkIdToConfig() returned error: %v", err)
	}

	networkId, err = GetNetworkIdFromConfig()
	if err != nil {
		t.Fatalf("GetNetworkIdFromConfig() after set returned error: %v", err)
	}
	if networkId != "net-456" {
		t.Fatalf("GetNetworkIdFromConfig() = %v, want net-456", networkId)
	}
}
