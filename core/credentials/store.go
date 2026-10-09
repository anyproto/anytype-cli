package credentials

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/localstate"
	"github.com/zalando/go-keyring"
)

const BackendEnv = "ANYTYPE_CLI_CREDENTIAL_BACKEND"

type storedValue struct {
	Secret  string `json:"secret,omitempty"`
	Backend string `json:"backend"`
}

// Store reads the existing CLI keyring without test writes. New secrets
// use the keyring when possible and a separate private, atomic file otherwise.
// The file is authoritative when a fallback was needed (a stale keyring entry
// must never shadow a newer credential).
type Store struct {
	Dir           string
	ConfigPath    string
	GetKeyring    func(string, string) (string, error)
	SetKeyring    func(string, string, string) error
	DeleteKeyring func(string, string) error
	// Backend is empty (automatic) or private_file. The explicit file selector
	// leaves unmarked legacy keyring entries alone; existing markers still win.
	Backend string
}

func New(dir, configPath string) *Store {
	return &Store{Dir: dir, ConfigPath: configPath, GetKeyring: keyring.Get, SetKeyring: keyring.Set, DeleteKeyring: keyring.Delete, Backend: os.Getenv(BackendEnv)}
}
func (s *Store) file(name string) string {
	sum := sha256.Sum256([]byte(name))
	return filepath.Join(s.Dir, "secrets", hex.EncodeToString(sum[:])+".json")
}
func (s *Store) Get(name string) (string, string, error) {
	selected, err := s.selectedBackend()
	if err != nil {
		return "", "", err
	}
	var value storedValue
	if err := privateRead(s.file(name), &value); err == nil {
		if value.Backend == "system_keyring" {
			v, err := s.GetKeyring("anytype-cli", name)
			return v, "system_keyring", err
		}
		if value.Backend == "private_file" {
			return value.Secret, "private_file", nil
		}
		return "", "", errors.New("unknown credential marker backend")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	var v string
	if selected != "private_file" {
		var err error
		v, err = s.GetKeyring("anytype-cli", name)
		if err != nil && !errors.Is(err, keyring.ErrNotFound) && !errors.Is(err, os.ErrNotExist) {
			return "", "system_keyring", err
		}
		if err == nil && v != "" {
			return v, "system_keyring", nil
		}
	}
	if name == "account-key" || name == "session-token" {
		c := new(config.Config)
		if err := localstate.ReadJSON(s.ConfigPath, c); err == nil {
			if name == "account-key" {
				v = c.AccountKey
			} else {
				v = c.SessionToken
			}
			if v != "" {
				return v, "legacy_config_file", nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", "legacy_config_file", err
		}
	}
	return "", "", os.ErrNotExist
}
func (s *Store) Set(name, value string) (string, error) {
	selected, err := s.selectedBackend()
	if err != nil {
		return "", err
	}
	var previous storedValue
	err = privateRead(s.file(name), &previous)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err == nil && previous.Backend != "private_file" && previous.Backend != "system_keyring" {
		return "", errors.New("unknown credential marker backend")
	}
	if s.Backend == "private_file" {
		// Persist the explicit instance choice on the first write. Service
		// auto-login and future operations then need no environment override.
		if err = localstate.WriteJSON(filepath.Join(s.Dir, "credential-backend.json"), storedValue{Backend: "private_file"}); err != nil {
			return "", err
		}
	}
	backend := "system_keyring"
	stored := ""
	if previous.Backend == "private_file" || selected == "private_file" && previous.Backend == "" {
		backend = "private_file"
		stored = value
	} else if err = s.SetKeyring("anytype-cli", name, value); err != nil {
		if previous.Backend == "system_keyring" {
			return "", err // Never silently migrate an existing keyring marker.
		}
		backend, stored = "private_file", value
	}
	err = localstate.WriteJSON(s.file(name), storedValue{stored, backend})
	return backend, err
}

func (s *Store) selectedBackend() (string, error) {
	if s.Backend != "" && s.Backend != "private_file" {
		return "", errors.New("unsupported credential backend selector")
	}
	var saved storedValue
	err := privateRead(filepath.Join(s.Dir, "credential-backend.json"), &saved)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err == nil {
		if saved.Backend != "private_file" {
			return "", errors.New("unsupported saved credential backend")
		}
		return saved.Backend, nil
	}
	return s.Backend, nil
}
func privateRead(path string, v any) error {
	return localstate.ReadPrivateJSON(path, v)
}

func (s *Store) Delete(name string) error {
	selected, err := s.selectedBackend()
	if err != nil {
		return err
	}
	var previous storedValue
	if err = privateRead(s.file(name), &previous); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// Commit the tombstone first, so temporary keyring failures cannot resurrect
	// the credential. Physical deletion and legacy config cleanup follow.
	if err := localstate.WriteJSON(s.file(name), struct {
		Secret  string `json:"secret"`
		Backend string `json:"backend"`
	}{"", "private_file"}); err != nil {
		return err
	}
	if s.DeleteKeyring != nil && (previous.Backend == "system_keyring" || previous.Backend == "" && selected != "private_file") {
		_ = s.DeleteKeyring("anytype-cli", name)
	}
	if name == "account-key" || name == "session-token" {
		data := map[string]any{}
		err := localstate.ReadJSON(s.ConfigPath, &data)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if name == "account-key" {
			delete(data, "accountKey")
		} else {
			delete(data, "sessionToken")
		}
		return localstate.WriteJSON(s.ConfigPath, data)
	}
	return nil
}
