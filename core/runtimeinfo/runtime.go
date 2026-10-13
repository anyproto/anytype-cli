// Package runtimeinfo records sanitized observations made by the CLI's own
// embedded server. Consumers must match Nonce against the live gRPC header;
// a file alone is never evidence of a running or authenticated account.
package runtimeinfo

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/localstate"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const Header = "anytype-cli-runtime"

type Snapshot struct {
	SchemaVersion     int    `json:"schema_version"`
	Nonce             string `json:"nonce"`
	PID               int    `json:"pid"`
	ConfigPath        string `json:"config_path"`
	DataPath          string `json:"data_path"`
	AccountId         string `json:"account_id,omitempty"`
	ProfileId         string `json:"profile_id,omitempty"`
	TechSpaceId       string `json:"tech_space_id,omitempty"`
	NetworkId         string `json:"network_id,omitempty"`
	APIAddress        string `json:"api_address,omitempty"`
	APIBound          bool   `json:"api_bound"`
	APIObservation    string `json:"api_observation,omitempty"`
	AccountTransition bool   `json:"account_transition"`
}

type Recorder struct {
	mu       sync.Mutex
	path     string
	snapshot Snapshot
	err      error
	// An in-process candidate only: never advertise it until the same account
	// and data root have been observed again. An accepted API event, rebind, or
	// stop invalidates it; ignored stale events cannot change retained evidence.
	previousAPI *Snapshot
	// Heart API events have no generation/account identity and may be delivered
	// after newer RPC outcomes. Only an unambiguous new startup address may use
	// an event as binding evidence; an RPC response always closes that window.
	startupAPIAddress string
	apiAddresses      map[string]bool
	apiUnknownAddress bool
}

func Path() string { return filepath.Join(localstate.Dir(), "runtime.json") }

func New(path, configPath, dataPath string) (*Recorder, error) {
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	r := &Recorder{path: path, snapshot: Snapshot{SchemaVersion: 1, Nonce: hex.EncodeToString(nonce[:]), PID: os.Getpid(), ConfigPath: configPath, DataPath: dataPath}}
	r.err = localstate.WriteJSON(path, r.snapshot)
	return r, r.err
}

func NewDefault() (*Recorder, error) {
	dataPath, err := filepath.Abs(config.GetDataDir())
	if err != nil {
		return nil, err
	}
	return New(Path(), config.GetConfigFilePath(), dataPath)
}
func (r *Recorder) Nonce() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return ""
	}
	return r.snapshot.Nonce
}
func (r *Recorder) update(fn func(*Snapshot)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(&r.snapshot)
	r.err = localstate.WriteJSON(r.path, r.snapshot)
}
func (r *Recorder) beginTransition(s *Snapshot) {
	r.startupAPIAddress = ""
	if !s.AccountTransition {
		r.previousAPI = nil
		if s.AccountId != "" {
			previous := *s
			r.previousAPI = &previous
		}
		s.APIBound = false
		s.APIAddress = ""
		s.APIObservation = "account_transition"
	}
	s.AccountTransition = true
}
func (r *Recorder) Transition() { r.update(r.beginTransition) }

// AccountSelection keeps an earlier binding only for a known continuation of
// the same account and data root. Heart can close the previous application
// while switching accounts, even when the selection later returns an error.
// Creation passes an empty identity: it always takes a destructive start path.
func (r *Recorder) AccountSelection(accountId string) {
	r.update(func(s *Snapshot) {
		r.beginTransition(s)
		previous := r.previousAPI
		if accountId == "" || previous == nil || previous.AccountId != accountId || previous.DataPath != s.DataPath {
			r.previousAPI = nil
		}
	})
}

// A failed response does not identify whether Heart kept the old app or closed
// it before startup failed. Neither retained evidence nor later unversioned
// events may establish continuity across that unknown outcome.
func (r *Recorder) AccountStartUncertain() {
	r.update(func(s *Snapshot) {
		r.previousAPI = nil
		r.startupAPIAddress = ""
		s.APIAddress, s.APIBound = "", false
		s.APIObservation = "account_start_outcome_unknown"
		s.AccountTransition = true
	})
}

// Stopping discards retained evidence before Heart closes components. A close
// error does not imply that the old app/listener remains available.
func (r *Recorder) Stopping() {
	r.update(func(s *Snapshot) {
		r.beginTransition(s)
		r.previousAPI = nil
		s.APIAddress, s.APIBound = "", false
		s.APIObservation = "stop_pending"
	})
}

func (r *Recorder) StopUncertain() {
	r.update(func(s *Snapshot) { s.APIObservation = "stop_outcome_unknown" })
}

func (r *Recorder) LifecycleUncertain() {
	r.update(func(s *Snapshot) {
		r.previousAPI = nil
		r.startupAPIAddress = ""
		s.APIAddress, s.APIBound = "", false
		s.APIObservation = "concurrent_lifecycle"
		// Account/wallet calls already mark their own transition. Overlapping
		// API-only rebinds leave a known account identity intact.
	})
}
func (r *Recorder) WalletTransition() {
	r.update(func(s *Snapshot) {
		r.beginTransition(s)
		s.AccountId = ""
		s.ProfileId = ""
		s.TechSpaceId = ""
		s.NetworkId = ""
	})
}
func (r *Recorder) Account(a *model.Account) {
	r.update(func(s *Snapshot) {
		if a == nil || a.Id == "" {
			return
		}
		if previous := r.previousAPI; previous != nil && previous.AccountId == a.Id && previous.DataPath == s.DataPath {
			s.APIAddress, s.APIBound = previous.APIAddress, previous.APIBound
			s.APIObservation = previous.APIObservation
			if !s.APIBound && (s.APIObservation == "" || s.APIObservation == "startup_pending") {
				s.APIObservation = "unverified_event_order"
			}
			r.startupAPIAddress = ""
		} else if !s.AccountTransition && s.AccountId != "" && s.AccountId != a.Id {
			s.APIAddress, s.APIBound = "", false
			s.APIObservation = "unverified_event_order"
		}
		r.previousAPI = nil
		s.AccountId = a.Id
		s.AccountTransition = false
		if a.Info != nil {
			s.ProfileId = a.Info.ProfileObjectId
			s.TechSpaceId = a.Info.TechSpaceId
			s.NetworkId = a.Info.NetworkId
		}
	})
}
func (r *Recorder) Stopped() {
	r.update(func(s *Snapshot) {
		r.previousAPI = nil
		r.startupAPIAddress = ""
		s.AccountId = ""
		s.ProfileId = ""
		s.TechSpaceId = ""
		s.NetworkId = ""
		s.APIBound = false
		s.APIAddress = ""
		s.APIObservation = "stopped"
		s.AccountTransition = false
	})
}
func (r *Recorder) API(address string, bound bool) {
	r.update(func(s *Snapshot) {
		r.previousAPI = nil
		r.startupAPIAddress = ""
		r.rememberAPIAddress(address)
		s.APIAddress = address
		s.APIBound = bound
		s.APIObservation = "rpc_response"
	})
}

// APIStarting opens a one-event window only when this address cannot identify
// an older startup/rebind in this process. Reusing an address without a retained
// same-account binding needs a direct rebind response or a fresh process.
func (r *Recorder) APIStarting(address string) {
	r.update(func(s *Snapshot) {
		r.startupAPIAddress = ""
		s.APIAddress, s.APIBound = "", false
		canonical := canonicalAPIAddress(address)
		unambiguous := canonical != "" && !r.apiUnknownAddress && !r.apiAddresses[canonical]
		r.rememberAPIAddress(address)
		s.APIObservation = "unverified_event_order"
		if address == "" {
			s.APIObservation = "startup_disabled"
		} else if unambiguous {
			r.startupAPIAddress = canonical
			s.APIObservation = "startup_pending"
		}
	})
}

// APIChanging invalidates prior evidence before a rebind can shut down the old
// listener. Even an error may follow shutdown, so a failed RPC stays unverified.
func (r *Recorder) APIChanging(address string) {
	r.update(func(s *Snapshot) {
		r.previousAPI = nil
		r.startupAPIAddress = ""
		r.rememberAPIAddress(address)
		s.APIAddress, s.APIBound = "", false
		s.APIObservation = "rpc_pending"
	})
}

func (r *Recorder) APIChangeUncertain() {
	r.update(func(s *Snapshot) { s.APIObservation = "rpc_outcome_unknown" })
}

func (r *Recorder) APIEvent(address string, bound bool) {
	r.update(func(s *Snapshot) {
		if r.startupAPIAddress == "" || canonicalAPIAddress(address) != r.startupAPIAddress {
			return
		}
		r.startupAPIAddress = ""
		r.previousAPI = nil
		s.APIAddress, s.APIBound = address, bound
		s.APIObservation = "startup_event"
	})
}

func canonicalAPIAddress(address string) string {
	host, port, err := net.SplitHostPort(address)
	p, portErr := strconv.Atoi(port)
	ip := net.ParseIP(host)
	if err != nil || portErr != nil || ip == nil || p < 1 || p > 65535 {
		return ""
	}
	if ip.IsUnspecified() {
		// Go's dual-stack tcp listener may report [::] for 0.0.0.0. These
		// wildcard spellings must share both event matching and history keys.
		return net.JoinHostPort("::", strconv.Itoa(p))
	}
	return net.JoinHostPort(ip.String(), strconv.Itoa(p))
}

// SameAPIAddress compares fixed numeric listener identities, including Go's
// dual-stack wildcard aliases. It does not select a client connection origin.
func SameAPIAddress(a, b string) bool {
	canonical := canonicalAPIAddress(a)
	return canonical != "" && canonical == canonicalAPIAddress(b)
}

func (r *Recorder) rememberAPIAddress(address string) {
	if address == "" {
		return
	}
	canonical := canonicalAPIAddress(address)
	if canonical == "" || len(r.apiAddresses) >= 1024 {
		// Dynamic/hostname listeners do not provide an exact address to fence.
		// Bound memory use under arbitrary native RPC callers conservatively.
		r.apiUnknownAddress = true
		return
	}
	if r.apiAddresses == nil {
		r.apiAddresses = make(map[string]bool)
	}
	r.apiAddresses[canonical] = true
}

// AccountActivity includes unresolved transitions. Auto-login must not race or
// override another caller's successful or interrupted account operation.
func (r *Recorder) AccountActivity() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshot.AccountId != "" || r.snapshot.AccountTransition
}

func (r *Recorder) DataPath(value string) {
	if value != "" {
		if absolute, err := filepath.Abs(value); err == nil {
			r.update(func(s *Snapshot) {
				if s.DataPath != absolute {
					r.previousAPI = nil
					r.startupAPIAddress = ""
					s.APIAddress, s.APIBound = "", false
					s.APIObservation = "unverified_event_order"
				}
				s.DataPath = absolute
			})
		}
	}
}
