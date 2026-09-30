package serviceprogram

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/output"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name          string
		apiListenAddr string
		wantAddr      string
	}{
		{
			name:          "with default address",
			apiListenAddr: config.DefaultAPIAddress,
			wantAddr:      config.DefaultAPIAddress,
		},
		{
			name:          "with custom address",
			apiListenAddr: "0.0.0.0:8080",
			wantAddr:      "0.0.0.0:8080",
		},
		{
			name:          "with empty address",
			apiListenAddr: "",
			wantAddr:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prg := New(tt.apiListenAddr)

			if prg == nil {
				t.Fatal("New() returned nil")
				return
			}

			if prg.apiListenAddr != tt.wantAddr {
				t.Errorf("apiListenAddr = %v, want %v", prg.apiListenAddr, tt.wantAddr)
			}

			if prg.startCh == nil {
				t.Error("startCh should be initialized")
			}
		})
	}
}

func TestGetService(t *testing.T) {
	svc, err := GetService()
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}

	if svc == nil {
		t.Fatal("GetService() returned nil service")
	}
}

func TestGetServiceWithAddress(t *testing.T) {
	tests := []struct {
		name    string
		apiAddr string
	}{
		{
			name:    "with empty address uses default",
			apiAddr: "",
		},
		{
			name:    "with default address",
			apiAddr: config.DefaultAPIAddress,
		},
		{
			name:    "with custom address",
			apiAddr: "0.0.0.0:9999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := GetServiceWithAddress(tt.apiAddr)
			if err != nil {
				t.Fatalf("GetServiceWithAddress() error = %v", err)
			}

			if svc == nil {
				t.Fatal("GetServiceWithAddress() returned nil service")
			}
		})
	}
}

func TestStartCallsOnStartedOnlyAfterServerStarts(t *testing.T) {
	tests := []struct {
		name        string
		startErr    error
		wantStarted bool
	}{
		{"server starts", nil, true},
		{"server fails to listen", errors.New("bind: address already in use"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(config.DefaultAPIAddress)
			p.startServer = func(grpcAddr, grpcWebAddr string) error { return tt.startErr }
			p.hasStoredAccount = func() bool { return true }
			started := false
			p.OnStarted = func() { started = true }

			err := p.Start(nil)
			if p.cancel != nil {
				p.cancel()
			}
			p.wg.Wait()

			if (err != nil) != (tt.startErr != nil) {
				t.Fatalf("Start() error = %v, want error %v", err, tt.startErr != nil)
			}
			if started != tt.wantStarted {
				t.Errorf("OnStarted called = %v, want %v", started, tt.wantStarted)
			}
		})
	}
}

// captureStdout returns what fn wrote to stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestStartupBanner(t *testing.T) {
	const url = "http://127.0.0.1:4000"

	withAccount := startupBanner(url, true)
	if len(withAccount) != 1 || withAccount[0] != "JSON API: "+url {
		t.Errorf("with stored account = %q, want just the address", withAccount)
	}

	without := strings.Join(startupBanner(url, false), "\n")
	for _, want := range []string{"JSON API: " + url, "anytype auth login", "anytype auth create <name>"} {
		if !strings.Contains(without, want) {
			t.Errorf("without stored account = %q, want it to contain %q", without, want)
		}
	}
}

func TestAutoLoginFailedBanner(t *testing.T) {
	got := strings.Join(autoLoginFailedBanner(), "\n")
	for _, want := range []string{"not running", "anytype auth login"} {
		if !strings.Contains(got, want) {
			t.Errorf("banner = %q, want it to contain %q", got, want)
		}
	}
}

func TestStartPrintsStartupBanner(t *testing.T) {
	tests := []struct {
		name       string
		startErr   error
		hasAccount bool
		want       string
	}{
		{"stored account", nil, true, output.FormatBanner(startupBanner("http://127.0.0.1:4000", true)...)},
		{"no stored account", nil, false, output.FormatBanner(startupBanner("http://127.0.0.1:4000", false)...)},
		{"server fails to listen", errors.New("bind: address already in use"), true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New("127.0.0.1:4000")
			p.startServer = func(grpcAddr, grpcWebAddr string) error { return tt.startErr }
			p.hasStoredAccount = func() bool { return tt.hasAccount }

			out := captureStdout(t, func() {
				_ = p.Start(nil)
				if p.cancel != nil {
					p.cancel()
				}
				p.wg.Wait()
			})

			if tt.want == "" {
				if strings.Contains(out, "JSON API") {
					t.Errorf("output %q mentions the JSON API although the server failed to start", out)
				}
				return
			}
			if !strings.Contains(out, tt.want) {
				t.Errorf("output %q does not contain banner %q", out, tt.want)
			}
		})
	}
}
