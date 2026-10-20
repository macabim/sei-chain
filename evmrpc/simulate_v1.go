package evmrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/export"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/sei-protocol/sei-chain/precompiles/wasmd"
	"github.com/sei-protocol/sei-chain/x/evm/state"
)

const maxSimulateV1Calls = 64

type SimulateV1Block struct {
	BlockOverrides *export.BlockOverrides   `json:"blockOverrides,omitempty"`
	StateOverrides *export.StateOverride    `json:"stateOverrides,omitempty"`
	Calls          []export.TransactionArgs `json:"calls"`
}

type SimulateV1Options struct {
	BlockStateCalls        []SimulateV1Block `json:"blockStateCalls"`
	Validation             bool              `json:"validation"`
	TraceTransfers         bool              `json:"traceTransfers"`
	ReturnFullTransactions bool              `json:"returnFullTransactions"`
}

type simulateV1Error struct {
	Code    int           `json:"code"`
	Message string        `json:"message"`
	Data    hexutil.Bytes `json:"data,omitempty"`
}

type SimulateV1CallResult struct {
	ReturnData hexutil.Bytes    `json:"returnData"`
	Logs       []*ethtypes.Log  `json:"logs"`
	GasUsed    hexutil.Uint64   `json:"gasUsed"`
	Status     hexutil.Uint64   `json:"status"`
	Error      *simulateV1Error `json:"error,omitempty"`
}

// SimulateV1 executes one bounded block of calls on isolated Sei state.
func (s *SimulationAPI) SimulateV1(ctx context.Context, opts SimulateV1Options, block *rpc.BlockNumberOrHash) (results []map[string]any, returnErr error) {
	ctx, cancel := simulationContext(ctx, s.backend.RPCEVMTimeout())
	defer cancel()
	start := time.Now()
	defer func() {
		returnErr = finishSimulation(ctx, "eth_simulateV1", s.connectionType, start, returnErr, recover())
	}()
	if len(opts.BlockStateCalls) != 1 {
		return nil, errors.New("eth_simulateV1 requires exactly one blockStateCalls entry")
	}
	input := opts.BlockStateCalls[0]
	if len(input.Calls) == 0 || len(input.Calls) > maxSimulateV1Calls {
		return nil, errors.New("eth_simulateV1 requires between 1 and 64 calls")
	}
	if opts.TraceTransfers || opts.ReturnFullTransactions {
		return nil, errors.New("eth_simulateV1 traceTransfers and returnFullTransactions are not supported")
	}
	if err := validateStateOverrides(input.StateOverrides, s.backend.MaxStateOverrideAccounts(), s.backend.MaxStateOverrideSlots()); err != nil {
		return nil, err
	}
	if s.requestLimiter != nil {
		if !s.requestLimiter.TryAcquire(1) {
			return nil, errors.New("eth_simulateV1 rejected due to rate limit: server busy")
		}
		defer s.requestLimiter.Release(1)
	}
	if block == nil {
		latest := rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber)
		block = &latest
	}
	db, base, err := s.backend.StateAndHeaderByNumberOrHash(ctx, *block)
	if err != nil {
		return nil, err
	}
	impl := state.GetDBImpl(db)
	defer impl.Cleanup()
	blockCtx, err := s.backend.keeper.GetVMBlockContext(impl.Ctx(), s.backend.keeper.GetGasPool())
	if err != nil {
		return nil, err
	}
	blockCtx.BlockNumber = new(big.Int).Add(base.Number, big.NewInt(1))
	blockCtx.Time = base.Time + 1
	blockCtx.GasLimit = base.GasLimit
	blockCtx.BaseFee = new(big.Int).Set(base.BaseFee)
	if !opts.Validation {
		blockCtx.BaseFee = new(big.Int)
	}
	if err := input.BlockOverrides.Apply(blockCtx); err != nil {
		return nil, err
	}
	if blockCtx.BlockNumber.Cmp(base.Number) <= 0 || !blockCtx.BlockNumber.IsInt64() || blockCtx.Time <= base.Time || blockCtx.Time > math.MaxInt64 {
		return nil, errors.New("simulation block number and time must follow the base block")
	}
	cap := s.backend.RPCGasCap()
	if cap == 0 {
		cap = 50_000_000
	}
	if blockCtx.GasLimit > cap {
		blockCtx.GasLimit = cap
	}
	// State overrides must not silently alter Sei precompile dispatch.
	if input.StateOverrides != nil {
		custom := s.backend.GetCustomPrecompiles(base.Number.Int64())
		standard := vm.ActivePrecompiles(s.backend.ChainConfig().Rules(base.Number, true, base.Time))
		for addr, override := range *input.StateOverrides {
			if override.MovePrecompileTo != nil {
				return nil, errors.New("moving precompiles is not supported")
			}
			for _, precompile := range standard {
				if precompile == addr && override.Code != nil {
					return nil, errors.New("overriding precompile code is not supported")
				}
			}
			if _, exists := custom[addr]; exists && override.Code != nil {
				return nil, errors.New("overriding Sei precompile code is not supported")
			}
		}
	}
	if err := input.StateOverrides.Apply(db, nil); err != nil {
		return nil, err
	}
	gp := new(core.GasPool).AddGas(blockCtx.GasLimit)
	calls := make([]SimulateV1CallResult, 0, len(input.Calls))
	hashes := make([]common.Hash, 0, len(input.Calls))
	var gasUsed uint64
	var logIndex uint
	for i, args := range input.Calls {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if args.BlobHashes != nil || args.Blobs != nil {
			return nil, errors.New("blob transactions are not supported on Sei")
		}
		if args.Gas != nil && uint64(*args.Gas) > gp.Gas() {
			return nil, errors.New("simulation gas limit exceeds remaining request budget")
		}
		from := common.Address{}
		if args.From != nil {
			from = *args.From
		}
		if args.Nonce == nil {
			nonce := hexutil.Uint64(db.GetNonce(from))
			args.Nonce = &nonce
		}
		if err := args.CallDefaults(gp.Gas(), blockCtx.BaseFee, s.backend.ChainConfig().ChainID); err != nil {
			return nil, err
		}
		msg := args.ToMessage(blockCtx.BaseFee, !opts.Validation, true)
		impl.WithCtx(impl.Ctx().WithContext(ctx).WithBlockHeight(blockCtx.BlockNumber.Int64()).WithBlockTime(time.Unix(int64(blockCtx.Time), 0)).WithEVMEntryViaWasmdPrecompile(wasmd.IsWasmdCall(args.To)))
		evm := s.backend.GetEVM(ctx, msg, db, base, &vm.Config{NoBaseFee: !opts.Validation}, blockCtx)
		execution, err := executeSimulateV1Call(ctx, evm, msg, gp)
		if err != nil {
			return nil, fmt.Errorf("simulation call %d: %w", i, err)
		}
		tx := args.ToTransaction(ethtypes.DynamicFeeTxType)
		hash := tx.Hash()
		call := SimulateV1CallResult{ReturnData: execution.Return(), Logs: []*ethtypes.Log{}, GasUsed: hexutil.Uint64(execution.UsedGas), Status: 1}
		if execution.Failed() {
			call.Status = 0
			call.Error = &simulateV1Error{Code: -32000, Message: execution.Err.Error()}
			if errors.Is(execution.Err, vm.ErrExecutionReverted) {
				call.Error.Code = 3
				call.Error.Data = execution.Revert()
				call.ReturnData = execution.Revert()
			}
		} else {
			call.Logs = impl.GetAllLogs()
			for _, log := range call.Logs {
				log.BlockNumber = blockCtx.BlockNumber.Uint64()
				log.TxHash = hash
				log.TxIndex = uint(i)
				log.Index = logIndex
				logIndex++
			}
		}
		gasUsed += execution.UsedGas
		calls = append(calls, call)
		hashes = append(hashes, hash)
		if err := impl.FinishSimulationTransaction(); err != nil {
			return nil, err
		}
	}
	header := ethtypes.CopyHeader(base)
	header.Number = blockCtx.BlockNumber
	header.Time = blockCtx.Time
	header.GasLimit = blockCtx.GasLimit
	header.GasUsed = gasUsed
	header.BaseFee = blockCtx.BaseFee
	header.Coinbase = blockCtx.Coinbase
	header.Difficulty = blockCtx.Difficulty
	if blockCtx.Random != nil {
		header.MixDigest = *blockCtx.Random
	}
	hash := header.Hash()
	for _, call := range calls {
		for _, log := range call.Logs {
			log.BlockHash = hash
		}
	}
	encoded, err := header.MarshalJSON()
	if err != nil {
		return nil, err
	}
	result := map[string]any{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, err
	}
	result["calls"] = calls
	result["transactions"] = hashes
	return []map[string]any{result}, nil
}

func executeSimulateV1Call(ctx context.Context, evm *vm.EVM, msg *core.Message, gp *core.GasPool) (*core.ExecutionResult, error) {
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			evm.Cancel()
		case <-done:
		}
	}()
	defer func() { close(done); <-stopped }()
	result, err := core.ApplyMessage(evm, msg, gp)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return result, err
}
