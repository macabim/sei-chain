package evmrpc_test

import (
	"context"

	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/sei-protocol/sei-chain/app"
	"github.com/sei-protocol/sei-chain/app/legacyabci"
	"github.com/sei-protocol/sei-chain/evmrpc"
	"github.com/sei-protocol/sei-chain/sei-cosmos/client"
	sdk "github.com/sei-protocol/sei-chain/sei-cosmos/types"
	"github.com/stretchr/testify/require"
)

func simulateV1Client(t *testing.T) *rpc.Client {
	t.Helper()
	a := app.Setup(t, false, false, false)
	base := a.GetContextForDeliverTx(nil).WithBlockHeight(MockHeight8)
	provider := func(int64) sdk.Context { return base }
	tm := NewMockClientWithLatest(MockHeight8)
	primeReceiptStore(t, a.EvmKeeper.ReceiptStore(), MockHeight8)
	watermarks := evmrpc.NewWatermarkManager(tm, provider, nil, a.EvmKeeper.ReceiptStore())
	api := evmrpc.NewSimulationAPI(provider, &a.EvmKeeper, legacyabci.BeginBlockKeepers{}, func(int64) client.TxConfig { return TxConfig }, tm,
		&evmrpc.SimulateConfig{GasCap: 1_000_000, EVMTimeout: 100 * time.Millisecond, MaxConcurrentSimulationCalls: 2, MaxStateOverrideAccounts: 8, MaxStateOverrideSlots: 16},
		a.BaseApp, a.TracerAnteHandler, evmrpc.ConnectionTypeHTTP, evmrpc.NewBlockCache(3000), &sync.Mutex{}, watermarks)
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", api))
	c := rpc.DialInProc(server)
	t.Cleanup(c.Close)
	t.Cleanup(server.Stop)
	return c
}

const simSender = "0x000000000000000000000000000000000000beef"
const simContract = "0x000000000000000000000000000000000000cafe"
const simStorageCode = "0x3615600c57600035600055005b60005460005260206000a060206000f3"

func simOptions(code string, data ...string) map[string]any {
	calls := make([]map[string]any, len(data))
	for i, input := range data {
		calls[i] = map[string]any{"from": simSender, "to": simContract, "data": input, "gas": "0x186a0"}
	}
	return map[string]any{"blockStateCalls": []any{map[string]any{"calls": calls, "stateOverrides": map[string]any{
		simSender:   map[string]any{"balance": "0x3635c9adc5dea00000"},
		simContract: map[string]any{"code": code, "state": map[string]string{}},
	}}}}
}

func callSim(t *testing.T, c *rpc.Client, opts map[string]any) []evmrpc.SimulateV1CallResult {
	t.Helper()
	var result []struct {
		Calls []evmrpc.SimulateV1CallResult `json:"calls"`
	}
	require.NoError(t, c.CallContext(t.Context(), &result, "eth_simulateV1", opts, "latest"))
	require.Len(t, result, 1)
	return result[0].Calls
}

func TestMacabimSimulateV1StorageAndIsolation(t *testing.T) {
	c := simulateV1Client(t)
	word := common.HexToHash("0x2a").Hex()
	calls := callSim(t, c, simOptions(simStorageCode, word, "0x"))
	require.Len(t, calls, 2)
	require.EqualValues(t, 1, calls[0].Status)
	require.EqualValues(t, 1, calls[1].Status)
	require.Equal(t, common.HexToHash(word).Bytes(), []byte(calls[1].ReturnData))
	require.Len(t, calls[1].Logs, 1)
	require.Equal(t, uint(1), calls[1].Logs[0].TxIndex)
	require.Equal(t, common.HexToHash(word).Bytes(), calls[1].Logs[0].Data)
	fresh := callSim(t, c, simOptions(simStorageCode, "0x"))
	require.Equal(t, make([]byte, 32), []byte(fresh[0].ReturnData))
	plain := map[string]any{"blockStateCalls": []any{map[string]any{"calls": []any{map[string]any{"to": simContract}}}}}
	require.Empty(t, callSim(t, c, plain)[0].ReturnData)
}

func TestMacabimSimulateV1TransientBoundaryAndReverts(t *testing.T) {
	c := simulateV1Client(t)
	// Store transient slot zero, then read it in a separate transaction.
	transient := "0x3615600c5760003560005d005b60005c60005260206000f3"
	calls := callSim(t, c, simOptions(transient, common.HexToHash("0x2a").Hex(), "0x"))
	require.EqualValues(t, 1, calls[0].Status)
	require.EqualValues(t, 1, calls[1].Status)
	require.Equal(t, make([]byte, 32), []byte(calls[1].ReturnData))
	reverted := callSim(t, c, simOptions("0x602a60005560006000fd", "0x", "0x"))
	for _, call := range reverted {
		require.Zero(t, call.Status)
		require.NotNil(t, call.Error)
		require.Empty(t, call.Logs)
	}
}

func TestMacabimSimulateV1LimitsAndCancellation(t *testing.T) {
	c := simulateV1Client(t)
	for _, opts := range []any{map[string]any{}, map[string]any{"blockStateCalls": []any{map[string]any{}}}} {
		var result any
		require.Error(t, c.CallContext(t.Context(), &result, "eth_simulateV1", opts, "latest"))
	}
	opts := simOptions(simStorageCode, "0x")
	opts["traceTransfers"] = true
	var result any
	require.Error(t, c.CallContext(t.Context(), &result, "eth_simulateV1", opts, "latest"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.Error(t, c.CallContext(ctx, &result, "eth_simulateV1", simOptions(simStorageCode, "0x"), "latest"))
	opts = simOptions(simStorageCode, "0x")
	entry := opts["blockStateCalls"].([]any)[0].(map[string]any)
	entry["calls"].([]map[string]any)[0]["gas"] = "0x1000000"
	require.ErrorContains(t, c.CallContext(t.Context(), &result, "eth_simulateV1", opts, "latest"), "budget")
	opts = simOptions(simStorageCode, "0x")
	entry = opts["blockStateCalls"].([]any)[0].(map[string]any)
	entry["calls"].([]map[string]any)[0]["nonce"] = "0xff"
	opts["validation"] = true
	require.ErrorContains(t, c.CallContext(t.Context(), &result, "eth_simulateV1", opts, "latest"), "nonce")
}
