package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Config struct {
	AccountId         string `json:"accountId,omitempty"`
	TechSpaceId       string `json:"techSpaceId,omitempty"`
	NetworkConfigPath string `json:"networkConfigPath,omitempty"`
	NetworkId         string `json:"networkId,omitempty"`
	// Credentials stored in plain text - only used when keyring is unavailable
	// WARNING: This is insecure and should only be used on headless servers
	AccountKey   string `json:"accountKey,omitempty"`
	SessionToken string `json:"sessionToken,omitempty"`
	// Settings are embedded so their keys stay at the top level of the file.
	Settings
}

// Settings are the user's choices about how this instance runs. Unlike the
// account fields in Config, they survive logout.
type Settings struct {
	// ApiListenAddr is the JSON API address last chosen with --listen-address.
	ApiListenAddr string `json:"apiListenAddr,omitempty"`
	// PreferTcp makes the account dial peers over TCP before QUIC.
	PreferTcp bool `json:"preferTcp,omitempty"`
}

var (
	instance *ConfigManager
	once     sync.Once
)

type ConfigManager struct {
	mu       sync.RWMutex
	config   *Config
	filePath string
}

func GetConfigManager() *ConfigManager {
	once.Do(func() {
		instance = &ConfigManager{
			config:   &Config{},
			filePath: GetConfigFilePath(),
		}
	})
	return instance
}

func (cm *ConfigManager) Load() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.filePath == "" {
		return fmt.Errorf("could not determine config file path")
	}

	data, err := os.ReadFile(cm.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Config doesn't exist yet, that's okay
			cm.config = &Config{}
			return nil
		}
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Decode into a fresh value: keys another process removed must not
	// survive from an earlier load.
	loaded := &Config{}
	if err := json.Unmarshal(data, loaded); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}
	cm.config = loaded

	return nil
}

func (cm *ConfigManager) Save() error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.filePath == "" {
		return fmt.Errorf("could not determine config file path")
	}

	configDir := filepath.Dir(cm.filePath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cm.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(cm.filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func (cm *ConfigManager) Get() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Return a copy to prevent external modification
	configCopy := *cm.config
	return &configCopy
}

func (cm *ConfigManager) GetFilePath() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.filePath
}

func (cm *ConfigManager) SetAccountId(accountId string) error {
	cm.mu.Lock()
	cm.config.AccountId = accountId
	cm.mu.Unlock()

	return cm.Save()
}

func (cm *ConfigManager) SetTechSpaceId(techSpaceId string) error {
	cm.mu.Lock()
	cm.config.TechSpaceId = techSpaceId
	cm.mu.Unlock()

	return cm.Save()
}

func (cm *ConfigManager) SetSessionToken(token string) error {
	cm.mu.Lock()
	cm.config.SessionToken = token
	cm.mu.Unlock()

	return cm.Save()
}

func (cm *ConfigManager) SetAccountKey(accountKey string) error {
	cm.mu.Lock()
	cm.config.AccountKey = accountKey
	cm.mu.Unlock()

	return cm.Save()
}

func (cm *ConfigManager) SetNetworkConfigPath(path string) error {
	cm.mu.Lock()
	cm.config.NetworkConfigPath = path
	cm.mu.Unlock()

	return cm.Save()
}

func (cm *ConfigManager) SetNetworkId(networkId string) error {
	cm.mu.Lock()
	cm.config.NetworkId = networkId
	cm.mu.Unlock()

	return cm.Save()
}

func (cm *ConfigManager) SetApiListenAddr(addr string) error {
	cm.mu.Lock()
	cm.config.ApiListenAddr = addr
	cm.mu.Unlock()

	return cm.Save()
}

// UpdateSettings applies fn to a copy of the settings and saves the result.
// Nothing changes when fn returns an error.
func (cm *ConfigManager) UpdateSettings(fn func(*Settings) error) error {
	cm.mu.Lock()
	settings := cm.config.Settings
	if err := fn(&settings); err != nil {
		cm.mu.Unlock()
		return err
	}
	cm.config.Settings = settings
	cm.mu.Unlock()

	return cm.Save()
}

// ClearAccount removes everything tied to the logged-in account and keeps
// settings. The file is deleted when no settings remain.
func (cm *ConfigManager) ClearAccount() error {
	cm.mu.Lock()
	keep := &Config{Settings: cm.config.Settings}
	cm.mu.Unlock()

	if keep.Settings == (Settings{}) {
		return cm.Delete()
	}

	cm.mu.Lock()
	cm.config = keep
	cm.mu.Unlock()
	return cm.Save()
}

func (cm *ConfigManager) Delete() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.filePath == "" {
		return fmt.Errorf("could not determine config file path")
	}

	cm.config = &Config{}

	if err := os.Remove(cm.filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete config file: %w", err)
	}

	return nil
}
