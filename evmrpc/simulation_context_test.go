package evmrpc_test

import (
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/sei-protocol/sei-chain/app"
	"github.com/sei-protocol/sei-chain/app/legacyabci"
	"github.com/sei-protocol/sei-chain/evmrpc"
	"github.com/sei-protocol/sei-chain/sei-cosmos/client"
	sdk "github.com/sei-protocol/sei-chain/sei-cosmos/types"
	"github.com/sei-protocol/sei-chain/x/evm/state"
	"github.com/stretchr/testify/require"
)

func TestSimulationStateCarriesCallDeadline(t *testing.T) {
	testApp := app.Setup(t, false, false, false)
	base := testApp.GetContextForDeliverTx(nil).WithBlockHeight(MockHeight8)
	provider := func(int64) sdk.Context { return base }
	tmClient := NewMockClientWithLatest(MockHeight8)
	primeReceiptStore(t, testApp.EvmKeeper.ReceiptStore(), MockHeight8)
	watermarks := evmrpc.NewWatermarkManager(tmClient, provider, nil, testApp.EvmKeeper.ReceiptStore())
	backend := evmrpc.NewBackend(provider, &testApp.EvmKeeper, legacyabci.BeginBlockKeepers{},
		func(int64) client.TxConfig { return TxConfig }, tmClient, &evmrpc.SimulateConfig{},
		testApp.BaseApp, testApp.TracerAnteHandler, evmrpc.NewBlockCache(3000), &sync.Mutex{}, watermarks)

	request, cancel := context.WithCancel(t.Context())
	defer cancel()
	db, header, err := backend.StateAndHeaderByNumberOrHash(request, rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber))
	require.NoError(t, err)
	impl := state.GetDBImpl(db)
	require.NotNil(t, impl)
	require.Same(t, request, impl.Ctx().Context())

	call, stop := context.WithTimeout(request, 20*time.Millisecond)
	defer stop()
	backend.GetEVM(call, &core.Message{GasPrice: big.NewInt(0)}, db, header, &vm.Config{}, &vm.BlockContext{BlockNumber: big.NewInt(MockHeight8)})
	require.Same(t, call, impl.Ctx().Context())
	<-call.Done()
	require.ErrorIs(t, impl.Ctx().Context().Err(), context.DeadlineExceeded)
	require.NoError(t, base.Context().Err(), "RPC cancellation must not cancel block execution")
}
