package policy

import (
	"github.com/morph-l2/go-ethereum/common"
)

// Config represents the configuration for the Policy Manager
type Config struct {
	WhitelistEnabled     bool             // Whether to enable whitelist policy
	WhitelistedAddresses []common.Address // List of whitelisted addresses
	MaxGasLimitSponsored uint64           // Maximum sponsored gas limit
}

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	return Config{
		WhitelistEnabled:     false,
		WhitelistedAddresses: []common.Address{},
		MaxGasLimitSponsored: 1_000_000,
	}
}
