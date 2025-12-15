package config

import (
	"fmt"
)

func GetAccountIdFromConfig() (string, error) {
	configMgr := GetConfigManager()
	if err := configMgr.Load(); err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}

	cfg := configMgr.Get()
	if cfg.AccountId == "" {
		return "", fmt.Errorf("no account Id found in config")
	}

	return cfg.AccountId, nil
}

func SetAccountIdToConfig(accountId string) error {
	configMgr := GetConfigManager()
	if err := configMgr.Load(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	return configMgr.SetAccountId(accountId)
}

func GetTechSpaceIdFromConfig() (string, error) {
	configMgr := GetConfigManager()
	if err := configMgr.Load(); err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}

	cfg := configMgr.Get()
	if cfg.TechSpaceId == "" {
		return "", fmt.Errorf("no tech space Id found in config")
	}

	return cfg.TechSpaceId, nil
}

func SetTechSpaceIdToConfig(techSpaceId string) error {
	configMgr := GetConfigManager()
	if err := configMgr.Load(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	return configMgr.SetTechSpaceId(techSpaceId)
}

func GetNetworkIdFromConfig() (string, error) {
	configMgr := GetConfigManager()
	if err := configMgr.Load(); err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}

	cfg := configMgr.Get()
	if cfg.NetworkId == "" {
		return "", fmt.Errorf("no network Id found in config")
	}

	return cfg.NetworkId, nil
}

func SetNetworkIdToConfig(networkId string) error {
	configMgr := GetConfigManager()
	if err := configMgr.Load(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	return configMgr.SetNetworkId(networkId)
}

func LoadStoredConfig() (*Config, error) {
	configMgr := GetConfigManager()
	if err := configMgr.Load(); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return configMgr.Get(), nil
}

func GetSessionTokenFromConfig() (string, error) {
	configMgr := GetConfigManager()
	if err := configMgr.Load(); err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}

	cfg := configMgr.Get()
	if cfg.SessionToken == "" {
		return "", fmt.Errorf("no session token found in config")
	}

	return cfg.SessionToken, nil
}

func SetSessionTokenToConfig(token string) error {
	configMgr := GetConfigManager()
	if err := configMgr.Load(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	return configMgr.SetSessionToken(token)
}

func GetAccountKeyFromConfig() (string, error) {
	configMgr := GetConfigManager()
	if err := configMgr.Load(); err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}

	cfg := configMgr.Get()
	if cfg.AccountKey == "" {
		return "", fmt.Errorf("no account key found in config")
	}

	return cfg.AccountKey, nil
}

func SetAccountKeyToConfig(accountKey string) error {
	configMgr := GetConfigManager()
	if err := configMgr.Load(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	return configMgr.SetAccountKey(accountKey)
}
