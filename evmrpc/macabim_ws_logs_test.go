package evmrpc_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestMacabimWSLogsParity checks the existing log fixtures over both transports.
func TestMacabimWSLogsParity(t *testing.T) {
	nonempty := 0
	for _, tt := range getCommonFilterLogTests() {
		t.Run(tt.name, func(t *testing.T) {
			criteria := map[string]interface{}{"address": tt.addrs, "topics": tt.topics}
			if tt.blockHash != nil {
				criteria["blockHash"] = tt.blockHash.Hex()
			}
			if tt.fromBlock != "" || tt.toBlock != "" {
				criteria["fromBlock"] = tt.fromBlock
				criteria["toBlock"] = tt.toBlock
			}
			expected := sendRequestGood(t, "getLogs", criteria)
			recv, done := sendWSRequestGood(t, "getLogs", criteria)
			defer close(done)
			select {
			case got, ok := <-recv:
				require.True(t, ok, "WebSocket closed before its response")
				require.Equal(t, expected["error"], got["error"])
				require.Equal(t, expected["result"], got["result"])
				if !tt.wantErr {
					require.Nil(t, got["error"])
					values, ok := got["result"].([]interface{})
					require.True(t, ok, "expected a log array")
					if len(values) > 0 {
						nonempty++
					}
					for _, value := range values {
						tt.check(t, value.(map[string]interface{}))
					}
				}
			case <-time.After(3 * time.Second):
				t.Fatal("WebSocket response timeout")
			}
		})
	}
	require.Positive(t, nonempty, "fixtures must contain logs")
}
