package policy

import "github.com/morph-l2/go-ethereum/common"

// WhitelistPolicy implements a whitelist-based policy
type WhitelistPolicy struct {
	name      string
	whitelist map[common.Address]bool
}

// NewWhitelistPolicy creates a new whitelist policy
func NewWhitelistPolicy(name string) *WhitelistPolicy {
	return &WhitelistPolicy{
		name:      name,
		whitelist: make(map[common.Address]bool),
	}
}

func (p *WhitelistPolicy) Name() string {
	return p.name
}

func (p *WhitelistPolicy) IsSponsorable(req SponsorableRequest) bool {
	return p.whitelist[req.From]
}

// AddToWhitelist adds an address to the whitelist
func (p *WhitelistPolicy) AddToWhitelist(address common.Address) {
	p.whitelist[address] = true
}

// RemoveFromWhitelist removes an address from the whitelist
func (p *WhitelistPolicy) RemoveFromWhitelist(address common.Address) {
	delete(p.whitelist, address)
}
