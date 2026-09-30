package config

// ResolveAPIListenAddr picks the JSON API address: an explicit --listen-address
// wins, then the address saved from an earlier explicit choice, then the
// default. The JSON API starts when an account logs in, on the address that
// login carries, so every command that logs in must agree on it.
func ResolveAPIListenAddr(flag string, flagSet bool, stored string) string {
	if flagSet {
		return flag
	}
	if stored != "" {
		return stored
	}
	return DefaultAPIAddress
}

// APIURL is the base URL of the JSON API listening on addr.
func APIURL(addr string) string {
	return "http://" + addr
}
