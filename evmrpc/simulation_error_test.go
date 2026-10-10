package evmrpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSimulationCancellationResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, recovered := range []any{nil, context.Canceled, context.DeadlineExceeded} {
		err := finishSimulation(ctx, "eth_call", ConnectionTypeHTTP, time.Now(), nil, recovered)
		require.ErrorIs(t, err, context.Canceled)
		require.Contains(t, err.Error(), "request timed out")
	}
	require.PanicsWithValue(t, "unexpected fault", func() {
		_ = finishSimulation(ctx, "eth_call", ConnectionTypeHTTP, time.Now(), nil, "unexpected fault")
	})
}

func TestSimulationContractError(t *testing.T) {
	err := errors.New("execution reverted")
	require.Same(t, err, finishSimulation(t.Context(), "eth_call", ConnectionTypeHTTP, time.Now(), err, nil))
}

func TestSimulationNoTimeout(t *testing.T) {
	ctx, cancel := simulationContext(t.Context(), 0)
	defer cancel()
	_, hasDeadline := ctx.Deadline()
	_, parentDeadline := t.Context().Deadline()
	require.Equal(t, parentDeadline, hasDeadline)
}
