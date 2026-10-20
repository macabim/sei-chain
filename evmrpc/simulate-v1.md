# Sei bundle simulation

`eth_simulateV1` runs an ordered target and backrun on isolated Sei state over HTTP or WebSocket.
It accepts one `blockStateCalls` entry with 1–64 calls. Calls share persistent state.
Each transaction gets fresh transient storage, access lists, refunds, logs, and original storage values.
Self-destruct cleanup runs between calls. No simulation cache writes to canonical state.

The block selector defaults to `latest`. Historical selectors use the existing Sei state resolver.
The default execution context is the next block, one second after the base block.
Sei transfer hooks and precompiles remain active. Non-validation mode defaults the base fee to zero, like Geth.
`validation: true` checks transaction nonces and fees. Neither mode checks signatures or guarantees inclusion.

State and block overrides use the existing call override types and account/slot limits.
Moving precompiles or overriding their code is rejected. Blob transactions are rejected.
`traceTransfers: true`, `returnFullTransactions: true`, multiple blocks, and empty call lists are rejected explicitly.
The response includes call status, gas used, return data, logs, and revert errors.
Transaction hashes describe unsigned simulation calls, not signed broadcast transactions.

The request shares the existing simulation concurrency limiter and timeout.
Total gas is bounded by the block limit and RPC gas cap, with a 50,000,000 fallback cap.
An explicit call gas limit above the remaining budget is rejected instead of silently reduced.
VM reverts produce failed call results. Invalid transactions or exhausted request limits fail the request.

Follow the [Macabim activation checks](https://github.com/macabim/Macabim/blob/codex/sei-mempool-support/docs/trader/simulation.md#sei-activation) before enabling mempool processing.
Also replay real pending swaps and verify that a subsequent call sees their effects.
Keep mempool trades disabled until the separate Macabim activation review completes.
