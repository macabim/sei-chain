package evmrpc

import (
	"context"
	"errors"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
)

type pendingTransactionSource interface {
	SubscribePendingTransactions() (<-chan common.Hash, func(), error)
}

// NewPendingTransactions subscribes to Ethereum hashes when local EVM transactions become ready.
func (a *SubscriptionAPI) NewPendingTransactions(ctx context.Context) (sub *rpc.Subscription, err error) {
	start := time.Now()
	defer func() {
		recordMetricsWithError(ctx, "eth_newPendingTransactions", a.connectionType, start, err, recover())
	}()
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return nil, rpc.ErrNotificationsUnsupported
	}
	source, supported := a.tmClient.(pendingTransactionSource)
	if !supported {
		return nil, errors.New("pending transaction subscriptions require a local mempool")
	}
	hashes, release, err := source.SubscribePendingTransactions()
	if err != nil {
		return nil, err
	}
	sub = notifier.CreateSubscription()
	// The request context ends after eth_subscribe returns. The subscription owns the worker.
	go func() {
		defer recoverAndLog()
		defer release()
		for {
			select {
			case <-sub.Err():
				return
			case hash, ok := <-hashes:
				if !ok {
					return
				}
				if err := notifier.Notify(sub.ID, hash); err != nil {
					return
				}
			}
		}
	}()
	return sub, nil
}
