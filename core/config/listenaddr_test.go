package config

import "testing"

func TestResolveAPIListenAddr(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		flagSet bool
		stored  string
		want    string
	}{
		{"explicit flag wins over stored", "0.0.0.0:5000", true, "127.0.0.1:4000", "0.0.0.0:5000"},
		{"explicit flag equal to default still wins", DefaultAPIAddress, true, "127.0.0.1:4000", DefaultAPIAddress},
		{"stored used when flag not set", DefaultAPIAddress, false, "127.0.0.1:4000", "127.0.0.1:4000"},
		{"default when nothing stored", DefaultAPIAddress, false, "", DefaultAPIAddress},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveAPIListenAddr(tt.flag, tt.flagSet, tt.stored); got != tt.want {
				t.Errorf("ResolveAPIListenAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAPIURL(t *testing.T) {
	tests := []struct {
		addr string
		want string
	}{
		{"127.0.0.1:31012", "http://127.0.0.1:31012"},
		{"0.0.0.0:31012", "http://0.0.0.0:31012"},
		{"[::1]:31012", "http://[::1]:31012"},
	}

	for _, tt := range tests {
		if got := APIURL(tt.addr); got != tt.want {
			t.Errorf("APIURL(%q) = %q, want %q", tt.addr, got, tt.want)
		}
	}
}
