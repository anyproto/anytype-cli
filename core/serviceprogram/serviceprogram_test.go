package serviceprogram

import (
	"testing"

	"github.com/anyproto/anytype-cli/core/config"
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
