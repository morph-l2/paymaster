package paymaster

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/morph-l2/go-ethereum/common"
	"github.com/morph-l2/go-ethereum/core/rawdb"
	"github.com/morph-l2/go-ethereum/core/state"
	"github.com/morph-l2/go-ethereum/core/types"
	"github.com/morph-l2/go-ethereum/params"
	"github.com/morph-l2/go-ethereum/rollup/rcfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockBlockchain struct {
	mock.Mock
}

func (m *mockBlockchain) State() (*state.StateDB, error) {
	args := m.Called()
	return args.Get(0).(*state.StateDB), args.Error(1)
}

func (m *mockBlockchain) Config() *params.ChainConfig {
	args := m.Called()
	return args.Get(0).(*params.ChainConfig)
}

func (m *mockBlockchain) CurrentHeader() *types.Header {
	args := m.Called()
	return args.Get(0).(*types.Header)
}

type mockOracle struct {
	mock.Mock
}

func (m *mockOracle) SuggestTipCap(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	return args.Get(0).(*big.Int), args.Error(1)
}

func TestPaymaster_SponsorPrice(t *testing.T) {
	tests := []struct {
		name          string
		userTxs       types.Transactions
		setupMocks    func(*mockBlockchain, *mockOracle, *state.StateDB)
		expectedPrice *big.Int
		expectedError bool
	}{
		{
			name: "Single zero gas price transaction",
			userTxs: types.Transactions{
				types.NewTransaction(0, common.Address{}, big.NewInt(0), 21000, big.NewInt(0), nil),
			},
			setupMocks: func(bc *mockBlockchain, o *mockOracle, s *state.StateDB) {
				header := &types.Header{
					BaseFee: big.NewInt(100),
					Number:  big.NewInt(1000),
				}
				bc.On("CurrentHeader").Return(header)
				bc.On("State").Return(s, nil)
				config := &params.ChainConfig{
					CurieBlock: big.NewInt(500), // Set Curie fork before current block
				}
				bc.On("Config").Return(config)

				// Setup L1 fee calculation parameters for Curie
				s.SetState(rcfg.L1GasPriceOracleAddress, rcfg.L1BaseFeeSlot, common.BigToHash(big.NewInt(1000)))
				s.SetState(rcfg.L1GasPriceOracleAddress, rcfg.L1BlobBaseFeeSlot, common.BigToHash(big.NewInt(100)))
				s.SetState(rcfg.L1GasPriceOracleAddress, rcfg.CommitScalarSlot, common.BigToHash(big.NewInt(1000000)))
				s.SetState(rcfg.L1GasPriceOracleAddress, rcfg.BlobScalarSlot, common.BigToHash(big.NewInt(500000)))

				o.On("SuggestTipCap", mock.Anything).Return(big.NewInt(50), nil)
			},
			// L2 gas cost = (baseFee + tipCap) * (sponsorGas + userGas) = (100+50) * (21000+21000) = 3150000
			// L1 data fee = singleTxL1Fee = 6000
			// Final sponsor price = (L2 gas cost + L1 data fee) / sponsorGas = (3150000 + 6000) / 21000 = 150.29 = 300
			expectedPrice: big.NewInt(300),
			expectedError: false,
		},
		{
			name: "Multiple zero gas price transactions",
			userTxs: types.Transactions{
				types.NewTransaction(0, common.Address{}, big.NewInt(0), 21000, big.NewInt(0), nil),
				types.NewTransaction(1, common.Address{}, big.NewInt(0), 30000, big.NewInt(0), nil),
			},
			setupMocks: func(bc *mockBlockchain, o *mockOracle, s *state.StateDB) {
				header := &types.Header{
					BaseFee: big.NewInt(100),
					Number:  big.NewInt(1000),
				}
				bc.On("CurrentHeader").Return(header)
				bc.On("State").Return(s, nil)
				config := &params.ChainConfig{
					CurieBlock: big.NewInt(500), // Set Curie fork before current block
				}
				bc.On("Config").Return(config)

				s.SetState(rcfg.L1GasPriceOracleAddress, rcfg.L1BaseFeeSlot, common.BigToHash(big.NewInt(1000)))
				s.SetState(rcfg.L1GasPriceOracleAddress, rcfg.L1BlobBaseFeeSlot, common.BigToHash(big.NewInt(100)))
				s.SetState(rcfg.L1GasPriceOracleAddress, rcfg.CommitScalarSlot, common.BigToHash(big.NewInt(1000000)))
				s.SetState(rcfg.L1GasPriceOracleAddress, rcfg.BlobScalarSlot, common.BigToHash(big.NewInt(500000)))

				o.On("SuggestTipCap", mock.Anything).Return(big.NewInt(50), nil)
			},
			// L2 gas cost = (baseFee + tipCap) * (sponsorGas + tx1Gas + tx2Gas) = (100+50) * (21000+21000+30000) = 10800000
			// L1 data fee = multipleTxL1Fee = 11000
			// Final sponsor price = (L2 gas cost + L1 data fee) / sponsorGas = (10800000 + 11000) / 21000 = 514.33 = 514
			expectedPrice: big.NewInt(514),
			expectedError: false,
		},
		{
			name: "No zero gas price transactions",
			userTxs: types.Transactions{
				types.NewTransaction(0, common.Address{}, big.NewInt(0), 21000, big.NewInt(1), nil),
			},
			setupMocks: func(bc *mockBlockchain, o *mockOracle, s *state.StateDB) {
				bc.On("State").Return(s, nil)
			},
			expectedPrice: nil,
			expectedError: false,
		},
		{
			name: "State error",
			userTxs: types.Transactions{
				types.NewTransaction(0, common.Address{}, big.NewInt(0), 21000, big.NewInt(0), nil),
			},
			setupMocks: func(bc *mockBlockchain, o *mockOracle, s *state.StateDB) {
				// Use a typed nil to avoid interface conversion issues
				bc.On("State").Return((*state.StateDB)(nil), errors.New("state error"))
			},
			expectedPrice: nil,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			blockchain := new(mockBlockchain)
			oracle := new(mockOracle)

			// Create a real state DB for testing unless it's a state error test
			var statedb *state.StateDB
			var err error
			if !tt.expectedError {
				db := state.NewDatabase(rawdb.NewMemoryDatabase())
				statedb, err = state.New(common.Hash{}, db, nil)
				require.NoError(t, err)
			}

			tt.setupMocks(blockchain, oracle, statedb)

			pm := &Paymaster{
				chain:  blockchain,
				oracle: oracle,
			}

			price, err := pm.sponsorPrice(context.Background(), tt.userTxs)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expectedPrice == nil {
					assert.Nil(t, price)
				} else {
					assert.Equal(t, 0, tt.expectedPrice.Cmp(price),
						"expected: %s, got: %s", tt.expectedPrice.String(), price.String())
				}
			}

			blockchain.AssertExpectations(t)
			oracle.AssertExpectations(t)
		})
	}
}
