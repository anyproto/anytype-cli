package core

import (
	"errors"
	"os"

	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/credentials"
	"github.com/anyproto/anytype-cli/core/localstate"
)

const (
	keyringService        = "anytype-cli"
	keyringTokenUser      = "session-token"
	keyringAccountKeyUser = "account-key"
)

var (
	keyringUnavailable = false
	ErrNotFound        = errors.New("credentials not found")
	credentialPaths    = func() (string, string) { return localstate.Dir(), config.GetConfigFilePath() }
)

func credentialStore() *credentials.Store {
	dir, path := credentialPaths()
	s := credentials.New(dir, path)
	if keyringUnavailable {
		s.GetKeyring = func(string, string) (string, error) { return "", os.ErrNotExist }
		s.SetKeyring = func(string, string, string) error { return os.ErrPermission }
		s.DeleteKeyring = func(string, string) error { return os.ErrPermission }
	}
	return s
}
func saveCredential(name, value string) (bool, error) {
	storage, err := credentialStore().Set(name, value)
	return storage == "system_keyring", err
}
func getCredential(name string) (string, bool, error) {
	value, storage, err := credentialStore().Get(name)
	if errors.Is(err, os.ErrNotExist) || err == nil && value == "" {
		return "", false, ErrNotFound
	}
	return value, storage == "system_keyring", err
}
func SaveSessionToken(token string) (bool, error)  { return saveCredential(keyringTokenUser, token) }
func SaveAccountKey(key string) (bool, error)      { return saveCredential(keyringAccountKeyUser, key) }
func GetStoredSessionToken() (string, bool, error) { return getCredential(keyringTokenUser) }
func GetStoredAccountKey() (string, bool, error)   { return getCredential(keyringAccountKeyUser) }
func DeleteStoredSessionToken() error              { return credentialStore().Delete(keyringTokenUser) }
func DeleteStoredAccountKey() error                { return credentialStore().Delete(keyringAccountKeyUser) }
