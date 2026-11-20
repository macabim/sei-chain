package evmrpc

import (
	"context"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/sei-protocol/sei-chain/sei-cosmos/client"
	"github.com/stretchr/testify/require"
)

type pendingTestClient struct {
	client.LocalClient
	hashes   chan common.Hash
	released chan struct{}
	once     sync.Once
}

func (c *pendingTestClient) SubscribePendingTransactions() (<-chan common.Hash, func(), error) {
	return c.hashes, func() { c.once.Do(func() { close(c.released) }) }, nil
}

func TestMacabimPendingRPCSubscription(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		name := "unsubscribe"
		if disconnect {
			name = "disconnect"
		}
		t.Run(name, func(t *testing.T) {
			source := &pendingTestClient{hashes: make(chan common.Hash, 1), released: make(chan struct{})}
			server := rpc.NewServer()
			require.NoError(t, server.RegisterName("eth", &SubscriptionAPI{tmClient: source, connectionType: ConnectionTypeWS}))
			defer server.Stop()
			rpcClient := rpc.DialInProc(server)
			defer rpcClient.Close()
			received := make(chan common.Hash, 1)
			ctx, cancel := context.WithCancel(t.Context())
			sub, err := rpcClient.EthSubscribe(ctx, received, "newPendingTransactions")
			require.NoError(t, err)
			cancel() // The request lifetime must not terminate the subscription.
			hash := common.HexToHash("0x1234")
			source.hashes <- hash
			select {
			case got := <-received:
				require.Equal(t, hash, got)
			case <-t.Context().Done():
				t.Fatal("notification missing")
			}
			if disconnect {
				rpcClient.Close()
			} else {
				sub.Unsubscribe()
			}
			select {
			case <-source.released:
			case <-t.Context().Done():
				t.Fatal("subscription leaked")
			}
		})
	}
}

func TestMacabimPendingRPCUnsupportedTransport(t *testing.T) {
	_, err := (&SubscriptionAPI{}).NewPendingTransactions(t.Context())
	require.ErrorIs(t, err, rpc.ErrNotificationsUnsupported)
}
