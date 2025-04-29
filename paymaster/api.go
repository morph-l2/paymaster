package paymaster

import (
	"context"

	"github.com/morph-l2/go-ethereum/paymaster/policy"
)

// PaymasterAPI provides RPC methods for the paymaster service
type PaymasterAPI struct {
	paymaster *Paymaster
}

// NewPaymasterAPI creates a new instance of PaymasterAPI
func NewPaymasterAPI(paymaster *Paymaster) *PaymasterAPI {
	return &PaymasterAPI{
		paymaster: paymaster,
	}
}

// IsSponsorable checks if a transaction is sponsorable
func (api *PaymasterAPI) IsSponsorable(ctx context.Context, req policy.SponsorableRequest) (*policy.SponsorableResponse, error) {
	return api.paymaster.IsSponsorable(ctx, req)
}
