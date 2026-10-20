package mempool

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sei-protocol/sei-chain/sei-tendermint/libs/utils"
)

const pendingSubscriberLimit = 128
const pendingSubscriberBuffer = 256

var errPendingSubscriberLimit = errors.New("pending transaction subscription limit reached")
var pendingSubscriptionDrops = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: MetricsNamespace, Subsystem: MetricsSubsystem,
	Name: "pending_subscription_dropped_total",
	Help: "Pending transaction notifications dropped because a subscriber buffer is full.",
})

// SubscribePendingTransactions returns new ready EVM hashes and an idempotent release function.
func (txmp *TxMempool) SubscribePendingTransactions() (<-chan common.Hash, func(), error) {
	return txmp.txStore.SubscribePendingTransactions()
}

// SubscribePendingTransactions returns new ready EVM hashes and an idempotent release function.
func (s *txStore) SubscribePendingTransactions() (<-chan common.Hash, func(), error) {
	for inner := range s.inner.Lock() {
		if len(inner.pendingSubscribers) >= pendingSubscriberLimit {
			return nil, func() {}, errPendingSubscriberLimit
		}
		ch := make(chan common.Hash, pendingSubscriberBuffer)
		inner.pendingSubscribers[ch] = struct{}{}
		release := func() {
			for inner := range s.inner.Lock() {
				if _, ok := inner.pendingSubscribers[ch]; ok {
					delete(inner.pendingSubscribers, ch)
					close(ch)
				}
			}
		}
		return ch, release, nil
	}
	panic("unreachable")
}

func (s *txStore) markReady(inner *txStoreInner, wtx *WrappedTx) {
	wtx.readyEl = utils.Some(s.readyTxs.PushBack(wtx.Tx()))
	if evm, ok := wtx.evm.Get(); ok {
		for ch := range inner.pendingSubscribers {
			// The store lock also protects lookup and unsubscribe. Never wait for RPC consumers.
			select {
			case ch <- evm.hash:
			default:
				pendingSubscriptionDrops.Inc()
			}
		}
	}
}
