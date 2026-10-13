package core

import (
	"strings"
	"testing"

	"github.com/anyproto/anytype-heart/pb"
)

func TestNewAccountSelectRequestPrefersYamuxTransport(t *testing.T) {
	req := newAccountSelectRequest(
		"account-id",
		"127.0.0.1:31012",
		"/tmp/anytype-data",
		pb.RpcAccount_CustomConfig,
		"/tmp/network.yml",
	)

	if !req.PreferYamuxTransport {
		t.Fatal("headless account selection must prefer Yamux/TCP to avoid QUIC invite timeouts")
	}
	if req.Id != "account-id" {
		t.Errorf("Id = %q, want %q", req.Id, "account-id")
	}
	if req.JsonApiListenAddr != "127.0.0.1:31012" {
		t.Errorf("JsonApiListenAddr = %q, want %q", req.JsonApiListenAddr, "127.0.0.1:31012")
	}
	if req.RootPath != "/tmp/anytype-data" {
		t.Errorf("RootPath = %q, want %q", req.RootPath, "/tmp/anytype-data")
	}
	if req.NetworkMode != pb.RpcAccount_CustomConfig {
		t.Errorf("NetworkMode = %v, want %v", req.NetworkMode, pb.RpcAccount_CustomConfig)
	}
	if req.NetworkCustomConfigFilePath != "/tmp/network.yml" {
		t.Errorf("NetworkCustomConfigFilePath = %q, want %q", req.NetworkCustomConfigFilePath, "/tmp/network.yml")
	}
}

func TestValidateAccountKey(t *testing.T) {
	tests := []struct {
		name        string
		accountKey  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid 64-byte account key (real example)",
			accountKey: "bNYSkBlOzNMKpDupAgL3g31Hnq7JpeX45O6MCpUqNdt16Avbgy5T5oQECKvAoy3+E4wHGPpCRCVWZQQCXRh7xw==",
			wantErr:    false,
		},
		{
			name:       "valid 32-byte account key (minimum)",
			accountKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			wantErr:    false,
		},
		{
			name:        "empty account key",
			accountKey:  "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "mnemonic instead of account key",
			accountKey:  "word1 word2 word3 word4 word5 word6 word7 word8 word9 word10 word11 word12",
			wantErr:     true,
			errContains: "appears to be a mnemonic phrase",
		},
		{
			name:        "not valid base64",
			accountKey:  "this-is-not-valid-base64!!!",
			wantErr:     true,
			errContains: "must be valid base64",
		},
		{
			name:        "base64 but insufficient key material",
			accountKey:  "QUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQQ==", // 30 bytes decoded
			wantErr:     true,
			errContains: "insufficient key material",
		},
		{
			name:        "very short base64",
			accountKey:  "YWJj", // "abc" decoded (3 bytes)
			wantErr:     true,
			errContains: "insufficient key material",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAccountKey(tt.accountKey)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAccountKey() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateAccountKey() error = %q, want to contain %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}
