package paymaster

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/morph-l2/go-ethereum/accounts/abi/bind"
	"github.com/morph-l2/go-ethereum/common"
	"github.com/morph-l2/go-ethereum/common/hexutil"
	"github.com/morph-l2/go-ethereum/core/state"
	"github.com/morph-l2/go-ethereum/core/types"
	"github.com/morph-l2/go-ethereum/crypto"
	"github.com/morph-l2/go-ethereum/log"
	"github.com/morph-l2/go-ethereum/params"
	"github.com/morph-l2/go-ethereum/paymaster/policy"
	"github.com/morph-l2/go-ethereum/rollup/fees"
)

type Oracle interface {
	SuggestTipCap(ctx context.Context) (*big.Int, error)
}

type TxPool interface {
	AllBundles() []*types.Bundle
	Pending(minTip *big.Int, baseFee *big.Int) map[common.Address]types.Transactions
	AddBundle(bundle *types.Bundle, originBundle *types.SendBundleArgs) error
}

type Blockchain interface {
	State() (*state.StateDB, error)
	Config() *params.ChainConfig
	CurrentHeader() *types.Header
}

var (
	ErrBundleGasLimitReached = errors.New("bundle gas limit reached")
	ErrBundleExistInvalidTxs = errors.New("bundle exists with invalid transactions")
	ErrInvalidSponsorTx      = errors.New("invalid sponsor transaction")
)

type Paymaster struct {
	config         Config
	policyMgr      *policy.PolicyManager
	txpool         TxPool
	chain          Blockchain
	oracle         Oracle
	signer         bind.SignerFn
	sponsorAddress common.Address

	ctx    context.Context
	cancel context.CancelFunc
	wg     *sync.WaitGroup
}

// New creates a new instance of Paymaster
func New(
	config Config,
	policyMgr *policy.PolicyManager,
	txpool TxPool,
	chain Blockchain,
	oracle Oracle,
) (*Paymaster, error) {
	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid paymaster config: %w", err)
	}

	privKey, err := crypto.HexToECDSA(config.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	transactor, err := bind.NewKeyedTransactorWithChainID(privKey, chain.Config().ChainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create transactor: %w", err)
	}

	txProcessorCtx, cancelTxProcessor := context.WithCancel(context.Background())

	return &Paymaster{
		config:         config,
		policyMgr:      policyMgr,
		signer:         transactor.Signer,
		sponsorAddress: transactor.From,
		txpool:         txpool,
		chain:          chain,
		oracle:         oracle,

		wg:     &sync.WaitGroup{},
		ctx:    txProcessorCtx,
		cancel: cancelTxProcessor,
	}, nil
}

func (pm *Paymaster) Start() {
	pm.RunTransactionProcessor()
}

func (pm *Paymaster) Close() {
	pm.cancel()
	pm.wg.Wait()
	log.Info("Paymaster processor stopped")
}

// IsSponsorable checks if a transaction is sponsorable
func (pm *Paymaster) IsSponsorable(ctx context.Context, req policy.SponsorableRequest) (*policy.SponsorableResponse, error) {
	return pm.policyMgr.IsSponsorable(ctx, req)
}

// RunTransactionProcessor starts a background process to send pending transactions
func (pm *Paymaster) RunTransactionProcessor() {
	pm.wg.Add(1)
	go func() {
		defer pm.wg.Done()
		ticker := time.NewTicker(pm.config.ProcessorInterval)
		defer ticker.Stop()
		for {
			select {
			case <-pm.ctx.Done():
				log.Info("Transaction processor shutting down")
				return
			case <-ticker.C:
				if err := pm.ProcessPendingTransactionsInBundles(pm.ctx); err != nil {
					log.Error("Failed to process pending transactions", "err", err)
				}
			}
		}
	}()
	log.Info("Paymaster processor started")
}

// ProcessPendingTransactionsInBundles processes pending transactions as bundles
func (pm *Paymaster) ProcessPendingTransactionsInBundles(ctx context.Context) error {
	bundles := pm.txpool.AllBundles()
	if len(bundles) > 0 {
		log.Info("Bundles exist in tx pool, waitting for them to be processed")
		for _, bundle := range bundles {
			log.Info("Pending bundle", "hash", bundle.Hash(), "sponsor tx nonce", bundle.Txs[0].Nonce())
		}
		return nil
	}

	pendingTxs := pm.txpool.Pending(nil, nil)
	if len(pendingTxs) == 0 {
		return nil
	}
	log.Info("Processing pending transactions in bundle", "count", len(pendingTxs))
	acculumatedGas := params.TxGas
	userTxs := make(types.Transactions, 0)
	for _, txs := range pendingTxs {
		for _, tx := range txs {
			if acculumatedGas+tx.Gas() > params.BundleGasLimit {
				continue
			}
			acculumatedGas += tx.Gas()
			userTxs = append(userTxs, tx)
		}
	}
	bundle, err := pm.createBundle(ctx, userTxs)
	if err != nil {
		log.Error("Failed to create bundle", "err", err)
		return err
	}

	rawTxs := make([]hexutil.Bytes, bundle.Txs.Len())
	for i, tx := range bundle.Txs {
		rawTxs[i], _ = tx.MarshalBinary()
	}

	hash := bundle.Hash()
	if err := pm.txpool.AddBundle(bundle, &types.SendBundleArgs{
		Txs: rawTxs,
	}); err != nil {
		log.Error("Failed to add bundle to txpool", "err", err, "bundle hash", hash)
		return err
	}

	log.Info("Bundle added to txpool", "bundle hash", hash, "sponsor tx nonce", bundle.Txs[0].Nonce())
	return nil
}

func (pm *Paymaster) createBundle(ctx context.Context, userTxs types.Transactions) (*types.Bundle, error) {
	sponsorPrice, err := pm.sponsorPrice(ctx, userTxs)
	if err != nil {
		log.Error("Failed to calculate sponsor price", "err", err)
	}
	if sponsorPrice == nil {
		log.Info("No user transactions need sponsoring, skipping sponsor transaction creation")
		return nil, nil
	}

	stateDB, _ := pm.chain.State()
	nonce := stateDB.GetNonce(pm.sponsorAddress)
	// Create sponsor transaction, a transfer transaction with a higher gas price to cover all user txs
	sponsorTx := types.NewTransaction(
		nonce,
		pm.sponsorAddress,
		big.NewInt(0), // Value - no transfer, just gas payment
		21000,
		sponsorPrice,
		nil,
	)
	signedTx, err := pm.signer(pm.sponsorAddress, sponsorTx)
	if err != nil {
		return nil, err
	}
	txs := make(types.Transactions, userTxs.Len()+1)
	txs[0] = signedTx
	copy(txs[1:], userTxs)

	return &types.Bundle{
		Txs: txs,
	}, nil
}

// sponsorPrice calculates the price of the sponsor transaction
func (pm *Paymaster) sponsorPrice(ctx context.Context, userTxs types.Transactions) (*big.Int, error) {
	// Calculate total gas needed for all transactions, including the sponsor transaction
	totalGasNeeded := uint64(0)
	totalUserTxsL1FeeCost := big.NewInt(0)

	stateDB, err := pm.chain.State()
	if err != nil {
		return nil, err
	}
	for _, tx := range userTxs {
		if tx.GasPrice().Cmp(big.NewInt(0)) == 0 {
			totalGasNeeded += tx.Gas()
			l1Fee, err := fees.CalculateL1DataFee(tx, stateDB, pm.chain.Config(), pm.chain.CurrentHeader().Number)
			if err != nil {
				return nil, err
			}
			totalUserTxsL1FeeCost.Add(totalUserTxsL1FeeCost, l1Fee)
		}
	}
	if totalGasNeeded == 0 {
		log.Info("No gas needed for user transactions, skipping bundle creation")
		return nil, nil
	}

	totalGasNeeded += 21000
	// Get current gas price
	tipCap, err := pm.oracle.SuggestTipCap(ctx)
	if err != nil {
		return nil, err
	}
	gasPrice := new(big.Int).Add(tipCap, pm.chain.CurrentHeader().BaseFee)

	totalL2FeeCost := new(big.Int).Mul(gasPrice, big.NewInt(int64(totalGasNeeded)))

	sponsorPrice := new(big.Int).Div(new(big.Int).Add(totalL2FeeCost, totalUserTxsL1FeeCost), big.NewInt(21000))

	return sponsorPrice, nil
}
