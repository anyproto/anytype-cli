package config

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

// PreferTcpEnv overrides the saved prefer-tcp setting in the process that
// starts the account, such as a Docker container or a foreground serve.
const PreferTcpEnv = "ANYTYPE_PREFER_TCP"

// Source says where a setting's effective value comes from.
type Source string

const (
	SourceDefault Source = "default"
	SourceConfig  Source = "config"
	SourceEnv     Source = "env"
)

// ResolvePreferTcp decides whether the account dials peers over TCP before
// QUIC: a non-empty ANYTYPE_PREFER_TCP wins, then the saved setting, then the
// default (QUIC first). An invalid environment value is skipped and returned
// as the error, so callers can warn without failing a login.
func ResolvePreferTcp(s Settings) (bool, Source, error) {
	var envErr error
	if env := os.Getenv(PreferTcpEnv); env != "" {
		value, err := strconv.ParseBool(env)
		if err == nil {
			return value, SourceEnv, nil
		}
		envErr = fmt.Errorf("ignoring %s=%q: use true or false", PreferTcpEnv, env)
	}
	if s.PreferTcp {
		return true, SourceConfig, envErr
	}
	return false, SourceDefault, envErr
}

// PreferTcp resolves the transport preference against the saved config. When
// the config can't be read, it resolves as if nothing were saved.
func PreferTcp() (bool, Source, error) {
	cfg, err := LoadStoredConfig()
	if err != nil {
		value, source, envErr := ResolvePreferTcp(Settings{})
		return value, source, errors.Join(err, envErr)
	}
	return ResolvePreferTcp(cfg.Settings)
}

// Setting is a key that `anytype config` shows.
type Setting struct {
	Key string
	// Env names the environment variable that overrides the saved value.
	Env string
	// RestartToApply means the running account keeps the old value until it
	// starts again.
	RestartToApply bool

	// readOnly, when set, is the error for `config set` and says how to
	// change the key instead.
	readOnly string
	resolve  func(Settings) (string, Source, error)
	// set and unset return the value they saved.
	set   func(*Settings, string) (string, error)
	unset func(*Settings) string
}

var settingKeys = []Setting{
	{
		Key:      "listen-address",
		readOnly: "set it with --listen-address on 'anytype service install', 'serve', 'auth login' or 'auth create' instead, so the service and the account agree on it",
		resolve: func(s Settings) (string, Source, error) {
			if s.ApiListenAddr == "" {
				return DefaultAPIAddress, SourceDefault, nil
			}
			return s.ApiListenAddr, SourceConfig, nil
		},
	},
	{
		Key:            "prefer-tcp",
		Env:            PreferTcpEnv,
		RestartToApply: true,
		resolve: func(s Settings) (string, Source, error) {
			value, source, err := ResolvePreferTcp(s)
			return strconv.FormatBool(value), source, err
		},
		set: func(s *Settings, value string) (string, error) {
			v, err := strconv.ParseBool(value)
			if err != nil {
				return "", fmt.Errorf("invalid value %q: use true or false", value)
			}
			s.PreferTcp = v
			return strconv.FormatBool(v), nil
		},
		unset: func(s *Settings) string {
			s.PreferTcp = false
			return "false"
		},
	},
}

// SettingKeys lists the keys `anytype config` knows, in display order.
func SettingKeys() []Setting {
	return slices.Clone(settingKeys)
}

// LookupSetting finds a key by name. The error lists the known keys.
func LookupSetting(key string) (Setting, error) {
	for _, s := range settingKeys {
		if s.Key == key {
			return s, nil
		}
	}
	keys := make([]string, len(settingKeys))
	for i, s := range settingKeys {
		keys[i] = s.Key
	}
	return Setting{}, fmt.Errorf("unknown config key %q (known keys: %s)", key, strings.Join(keys, ", "))
}

// Resolve returns the effective value and where it comes from. A non-nil
// error is a warning about an ignored override; the value is still usable.
func (s Setting) Resolve(stored Settings) (string, Source, error) {
	return s.resolve(stored)
}

// Settable reports whether `config set` and `config unset` may change the key.
func (s Setting) Settable() bool {
	return s.set != nil
}

// Set validates value, stores it in settings and returns it as saved.
func (s Setting) Set(settings *Settings, value string) (string, error) {
	if !s.Settable() {
		return "", errors.New(s.readOnly)
	}
	return s.set(settings, value)
}

// Unset returns the key to its default and returns that default.
func (s Setting) Unset(settings *Settings) (string, error) {
	if !s.Settable() {
		return "", errors.New(s.readOnly)
	}
	return s.unset(settings), nil
}
