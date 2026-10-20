package state_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/holiman/uint256"
	testkeeper "github.com/sei-protocol/sei-chain/testutil/keeper"
	"github.com/sei-protocol/sei-chain/x/evm/state"
	"github.com/stretchr/testify/require"
)

func TestSimulationTransactionBoundary(t *testing.T) {
	k := &testkeeper.EVMTestApp.EvmKeeper
	ctx := testkeeper.EVMTestApp.GetContextForDeliverTx(nil)
	_, addr := testkeeper.MockAddressPair()
	db := state.NewDBImpl(ctx, k, true)
	key, value := common.HexToHash("0x1"), common.HexToHash("0x2a")
	db.SetState(addr, key, value)
	db.SetTransientState(addr, key, value)
	db.AddRefund(100)
	db.AddAddressToAccessList(addr)
	require.NoError(t, db.FinishSimulationTransaction())
	require.Equal(t, value, db.GetState(addr, key))
	require.Equal(t, value, db.GetCommittedState(addr, key))
	require.Equal(t, common.Hash{}, db.GetTransientState(addr, key))
	require.Zero(t, db.GetRefund())
	require.False(t, db.AddressInAccessList(addr))
	require.Equal(t, common.Hash{}, state.NewDBImpl(ctx, k, true).GetState(addr, key))
	db.CreateAccount(addr)
	db.SetCode(addr, []byte{0x00})
	db.AddBalance(addr, uint256.NewInt(1), tracing.BalanceChangeUnspecified)
	db.SelfDestruct(addr)
	require.NoError(t, db.FinishSimulationTransaction())
	require.Empty(t, db.GetCode(addr))
	require.Equal(t, common.Hash{}, db.GetState(addr, key))
	require.Error(t, state.NewDBImpl(ctx, k, false).FinishSimulationTransaction())
}
