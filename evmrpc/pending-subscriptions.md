# Pending transaction subscriptions

The Macabim legacy node exposes `eth_subscribe` with `["newPendingTransactions"]` on the native WebSocket endpoint.
Each `eth_subscription` notification contains an Ethereum transaction hash as a `0x`-prefixed, 32-byte string.
Use `eth_getTransactionByHash` over HTTP or WebSocket to obtain the transaction.
Pending transaction responses have null block fields. A transaction can be included before lookup completes.

The feed emits validated EVM transactions when they become ready in the local mempool.
It includes peer transactions, ready replacements, and transactions released by a filled nonce gap.
It excludes Cosmos transactions, rejected duplicates, and transactions that remain queued.
It does not replay the pool when a client connects. Inclusion is not guaranteed.

The node permits 128 concurrent pending subscriptions. Each subscription buffers 256 hashes.
A full buffer drops new notifications and increments `tendermint_mempool_pending_subscription_dropped_total`.
Slow clients cannot block mempool admission. Unsubscribe and disconnect release the buffer and worker.
The feed is best effort; consumers must tolerate gaps and reconnects.

Autobahn nodes without this legacy mempool reject the subscription.
Full transaction notifications and HTTP pending filters are not added by this patch.
This feed does not enable Tamnun or Trader mempool processing.
