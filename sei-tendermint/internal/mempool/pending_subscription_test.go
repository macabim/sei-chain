package mempool

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/sei-protocol/sei-chain/sei-tendermint/internal/proxy"
	"github.com/sei-protocol/sei-chain/sei-tendermint/libs/utils"
	"github.com/sei-protocol/sei-chain/sei-tendermint/libs/utils/require"
)

func TestPendingSubscriptionsReadyAndReplacement(t *testing.T) {
	rng := utils.TestRng()
	app := newEVMNonceApp()
	store := NewTxStore(TestConfig(), proxy.New(app))
	address := common.HexToAddress("0x1234")
	app.setBalance(address, 100)
	ch, release, err := store.SubscribePendingTransactions()
	require.NoError(t, err)
	defer release()
	// A nonce gap is not ready. Filling it publishes both transactions in nonce order.
	later := makeEvmTxForTest(rng, address, 1, 1, 1)
	require.NoError(t, store.Insert(later))
	select {
	case <-ch:
		t.Fatal("queued transaction published")
	default:
	}
	first := makeEvmTxForTest(rng, address, 0, 1, 1)
	require.NoError(t, store.Insert(first))
	for _, tx := range []*WrappedTx{first, later} {
		hash, err := utils.Recv(t.Context(), ch)
		require.NoError(t, err)
		require.Equal(t, tx.evm.OrPanic("evm").hash, hash)
		raw, found := store.ByEvmHash(hash)
		require.True(t, found)
		require.Equal(t, string(tx.Tx()), string(raw))
	}
	require.Error(t, store.Insert(first))
	select {
	case <-ch:
		t.Fatal("duplicate published")
	default:
	}
	replacement := makeEvmTxForTest(rng, address, 0, 2, 1)
	require.NoError(t, store.Insert(replacement))
	hash, err := utils.Recv(t.Context(), ch)
	require.NoError(t, err)
	require.Equal(t, replacement.evm.OrPanic("evm").hash, hash)
	require.NoError(t, store.Insert(&WrappedTx{hashedTx: newHashedTx([]byte("cosmos")), priority: 1}))
	select {
	case <-ch:
		t.Fatal("Cosmos transaction published")
	default:
	}
}

func TestPendingSubscriptionsBoundedAndRelease(t *testing.T) {
	store := newTxStoreForTest()
	var releases []func()
	for range pendingSubscriberLimit {
		_, release, err := store.SubscribePendingTransactions()
		require.NoError(t, err)
		releases = append(releases, release)
	}
	_, _, err := store.SubscribePendingTransactions()
	require.Error(t, err)
	for _, release := range releases {
		release()
		release()
	}
	ch, release, err := store.SubscribePendingTransactions()
	require.NoError(t, err)
	release()
	_, open := <-ch
	require.False(t, open)
}

func TestPendingSubscriptionsSlowConsumerDoesNotBlock(t *testing.T) {
	rng := utils.TestRng()
	app := newEVMNonceApp()
	store := NewTxStore(TestConfig(), proxy.New(app))
	address := common.HexToAddress("0x1234")
	app.setBalance(address, 100)
	slow, releaseSlow, err := store.SubscribePendingTransactions()
	require.NoError(t, err)
	defer releaseSlow()
	fast, releaseFast, err := store.SubscribePendingTransactions()
	require.NoError(t, err)
	defer releaseFast()
	for i := range pendingSubscriberBuffer + 10 {
		tx := makeEvmTxForTest(rng, address, 0, int64(i+1), 1)
		require.NoError(t, store.Insert(tx))
		hash, err := utils.Recv(t.Context(), fast)
		require.NoError(t, err)
		require.Equal(t, tx.evm.OrPanic("evm").hash, hash)
	}
	require.Equal(t, pendingSubscriberBuffer, len(slow))
}
