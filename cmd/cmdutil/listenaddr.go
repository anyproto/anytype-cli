package cmdutil

import (
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/core/config"
)

const listenAddressFlag = "listen-address"

// storedAPIListenAddr reads the saved JSON API address; replaceable in tests.
var storedAPIListenAddr = config.GetApiListenAddrFromConfig

// AddListenAddressFlag registers --listen-address for the JSON API.
func AddListenAddressFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, listenAddressFlag, config.DefaultAPIAddress,
		"JSON API listen address in `host:port` format (remembered for later commands)")
}

// APIListenAddr resolves the JSON API address for a command and reports
// whether the user passed --listen-address explicitly.
func APIListenAddr(cmd *cobra.Command, flagValue string) (string, bool) {
	changed := cmd.Flags().Changed(listenAddressFlag)
	stored, _ := storedAPIListenAddr()
	return config.ResolveAPIListenAddr(flagValue, changed, stored), changed
}
