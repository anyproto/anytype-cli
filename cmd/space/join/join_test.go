package join

import "testing"

func TestParseInviteLink(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCid   string
		wantKey   string
		expectErr bool
	}{
		{
			name:    "default host",
			input:   "https://invite.any.coop/abc123#filekey",
			wantCid: "abc123",
			wantKey: "filekey",
		},
		{
			name:    "custom host and nested path",
			input:   "https://selfhost.local/invites/space-1#k1",
			wantCid: "space-1",
			wantKey: "k1",
		},
		{
			name:      "missing fragment",
			input:     "https://selfhost.local/invites/space-1",
			expectErr: true,
		},
		{
			name:      "missing cid",
			input:     "https://selfhost.local/#k1",
			expectErr: true,
		},
		{
			name:      "unsupported scheme",
			input:     "ftp://selfhost.local/space#k1",
			expectErr: true,
		},
		{
			name:      "missing host",
			input:     "https:///space#k1",
			expectErr: true,
		},
		{
			name:    "http scheme allowed",
			input:   "http://selfhost.local/space#k1",
			wantCid: "space",
			wantKey: "k1",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			cid, key, err := parseInviteLink(tt.input)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cid != tt.wantCid {
				t.Fatalf("cid = %s, want %s", cid, tt.wantCid)
			}
			if key != tt.wantKey {
				t.Fatalf("key = %s, want %s", key, tt.wantKey)
			}
		})
	}
}
