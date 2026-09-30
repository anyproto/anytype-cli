//go:build !nogrpcserver

package grpcserver

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

// captureStderr returns what fn wrote to stderr. zap opens its "stderr" sink
// when the config is applied, so fn must apply it after the swap.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestApplyLogLevelFiltersBeforeLogin(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		wantInfo  bool
		wantError bool
	}{
		{"default is ERROR", "", false, true},
		{"quiet mode", "*=FATAL", false, false},
		{"verbose mode", "*=DEBUG", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ANYTYPE_LOG_LEVEL", tt.env)
			log := logging.Logger("anytype-cli-loglevel-test")

			out := captureStderr(t, func() {
				applyLogLevel()
				log.Info("INFO-LINE")
				log.Error("ERROR-LINE")
				_ = log.Sync()
			})

			if got := strings.Contains(out, "INFO-LINE"); got != tt.wantInfo {
				t.Errorf("info shown = %v, want %v (output %q)", got, tt.wantInfo, out)
			}
			if got := strings.Contains(out, "ERROR-LINE"); got != tt.wantError {
				t.Errorf("error shown = %v, want %v (output %q)", got, tt.wantError, out)
			}
		})
	}
}
