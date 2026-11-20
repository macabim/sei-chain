package evmrpc_test

import (
	"encoding/json"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/sei-protocol/sei-chain/evmrpc"
	"github.com/sei-protocol/sei-chain/sei-cosmos/client"
	sdk "github.com/sei-protocol/sei-chain/sei-cosmos/types"
	"github.com/sei-protocol/sei-chain/sei-tendermint/libs/utils"
	tmtypes "github.com/sei-protocol/sei-chain/sei-tendermint/types"
	"github.com/sei-protocol/sei-chain/x/evm/types"
	"github.com/sei-protocol/sei-chain/x/evm/types/ethtx"
	"github.com/stretchr/testify/require"
)

type pendingFormatClient struct {
	client.LocalClient
	raw tmtypes.Tx
}

func (c *pendingFormatClient) EvmTxByHash(common.Hash) (tmtypes.Tx, bool) { return c.raw, true }

func TestMacabimPendingTransactionFormat(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	chainID := EVMKeeper.ChainID(Ctx)
	to := common.HexToAddress("0x1234")
	accessList := ethtypes.AccessList{{Address: to, StorageKeys: []common.Hash{common.HexToHash("0x01")}}}
	cases := []struct {
		name string
		data ethtypes.TxData
	}{
		{"legacy", &ethtypes.LegacyTx{Nonce: 3, GasPrice: big.NewInt(20), Gas: 50000, To: &to, Value: big.NewInt(2), Data: []byte{1, 2}}},
		{"access-list", &ethtypes.AccessListTx{ChainID: chainID, Nonce: 3, GasPrice: big.NewInt(20), Gas: 50000, To: &to, Value: big.NewInt(2), Data: []byte{1, 2}, AccessList: accessList}},
		{"dynamic-fee", &ethtypes.DynamicFeeTx{ChainID: chainID, Nonce: 3, GasFeeCap: big.NewInt(20), GasTipCap: big.NewInt(2), Gas: 50000, To: &to, Value: big.NewInt(2), Data: []byte{1, 2}, AccessList: accessList}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			signed, err := ethtypes.SignNewTx(key, ethtypes.LatestSignerForChainID(chainID), tc.data)
			require.NoError(t, err)
			data, err := ethtx.NewTxDataFromTx(signed)
			require.NoError(t, err)
			msg, err := types.NewMsgEVMTransaction(data)
			require.NoError(t, err)
			builder := TxConfig.NewTxBuilder()
			require.NoError(t, builder.SetMsgs(msg))
			raw, err := TxConfig.TxEncoder()(builder.GetTx())
			require.NoError(t, err)
			api := evmrpc.NewTransactionAPI(&pendingFormatClient{raw: raw}, EVMKeeper,
				func(int64) sdk.Context { return Ctx }, func(int64) client.TxConfig { return TxConfig },
				"", evmrpc.ConnectionTypeHTTP, utils.None[time.Duration](), &evmrpc.WatermarkManager{}, evmrpc.NewBlockCache(1), &sync.Mutex{})
			result, err := api.GetTransactionByHash(t.Context(), signed.Hash())
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Nil(t, result.BlockHash)
			require.Nil(t, result.BlockNumber)
			require.Nil(t, result.TransactionIndex)
			require.Equal(t, crypto.PubkeyToAddress(key.PublicKey), result.From)
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			var decoded ethtypes.Transaction
			require.NoError(t, decoded.UnmarshalJSON(encoded))
			require.Equal(t, signed.Hash(), decoded.Hash())
			require.Equal(t, signed.AccessList(), decoded.AccessList())
			require.Equal(t, signed.GasFeeCap(), decoded.GasFeeCap())
			require.Equal(t, signed.GasTipCap(), decoded.GasTipCap())
		})
	}
}
