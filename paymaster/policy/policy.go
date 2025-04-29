package policy

import (
	"context"
	"math/big"

	"github.com/morph-l2/go-ethereum/common"
	"github.com/morph-l2/go-ethereum/common/hexutil"
	"github.com/morph-l2/go-ethereum/core/types"
	"github.com/morph-l2/go-ethereum/log"
)

// Policy represents a sponsorship policy
type Policy interface {
	Name() string
	IsSponsorable(req SponsorableRequest) bool
}

// SponsorableRequest represents the request parameters for pm_isSponsorable
type SponsorableRequest struct {
	To    common.Address `json:"to"`
	From  common.Address `json:"from"`
	Value *hexutil.Big   `json:"value"`
	Data  hexutil.Bytes  `json:"data"`
	Gas   *hexutil.Big   `json:"gas"`
}

// SponsorableResponse represents the response from pm_isSponsorable
type SponsorableResponse struct {
	Sponsorable   bool   `json:"Sponsorable"`
	SponsorPolicy string `json:"SponsorPolicy"`
}

// PolicyManager manages paymaster policies
type PolicyManager struct {
	policies map[string]Policy
	config   Config
	signer   types.Signer
}

// NewPolicyManager creates a new policy manager
func NewPolicyManager(config Config, signer types.Signer) *PolicyManager {
	policies := make(map[string]Policy)

	// Add default policy
	defaultPolicy := &DefaultPolicy{
		name: "default",
	}
	policies[defaultPolicy.Name()] = defaultPolicy

	if config.WhitelistEnabled {
		whitelistPolicy := &WhitelistPolicy{
			name: "whitelist",
		}
		for _, address := range config.WhitelistedAddresses {
			whitelistPolicy.AddToWhitelist(address)
		}
		policies[whitelistPolicy.Name()] = whitelistPolicy
	}
	return &PolicyManager{
		policies: policies,
		config:   config,
		signer:   signer,
	}
}

func (pm *PolicyManager) Sponsorable(tx *types.Transaction) (bool, error) {
	from, _ := types.Sender(pm.signer, tx)
	resp, err := pm.IsSponsorable(context.Background(), SponsorableRequest{
		To:    *tx.To(),
		From:  from,
		Value: (*hexutil.Big)(tx.Value()),
		Data:  tx.Data(),
		Gas:   (*hexutil.Big)(big.NewInt(int64(tx.Gas()))),
	})
	if err != nil {
		return false, err
	}
	return resp.Sponsorable, nil
}

func (pm *PolicyManager) IsSponsorable(ctx context.Context, req SponsorableRequest) (*SponsorableResponse, error) {
	log.Info("Checking if transaction is sponsorable",
		"from", req.From,
		"to", req.To,
		"value", req.Value)

	// Check if gas limit exceeds the limit in configuration
	if req.Gas != nil && req.Gas.ToInt().Uint64() > pm.config.MaxGasLimitSponsored {
		log.Warn("Gas limit exceeds maximum allowed",
			"requested", req.Gas.ToInt().Uint64(),
			"max", pm.config.MaxGasLimitSponsored)
		return &SponsorableResponse{
			Sponsorable:   false,
			SponsorPolicy: "",
		}, nil
	}

	// Check all policies and find the first one that allows sponsoring
	for _, policy := range pm.GetPolicies() {
		if policy.IsSponsorable(req) {
			return &SponsorableResponse{
				Sponsorable:   true,
				SponsorPolicy: policy.Name(),
			}, nil
		}
	}

	// If no policy allows sponsoring, return false
	return &SponsorableResponse{
		Sponsorable:   false,
		SponsorPolicy: "",
	}, nil
}

// RegisterPolicy registers a new policy
func (pm *PolicyManager) RegisterPolicy(policy Policy) {
	pm.policies[policy.Name()] = policy
}

// GetPolicies returns all registered policies
func (pm *PolicyManager) GetPolicies() map[string]Policy {
	return pm.policies
}

// GetPolicy returns a policy by name
func (pm *PolicyManager) GetPolicy(name string) Policy {
	return pm.policies[name]
}
