package state

import "errors"

// FinishSimulationTransaction closes a transaction on an isolated simulation state.
func (s *DBImpl) FinishSimulationTransaction() error {
	if !s.simulation {
		return errors.New("simulation transaction boundary requires simulation state")
	}
	if s.err != nil {
		return s.err
	}
	s.handleResidualFundsInDestructedAccounts(s.tempState)
	s.clearAccountStateIfDestructed(s.tempState)
	// Keep full-storage overrides across calls, but advance their original values.
	overrides := s.tempState.storageOverrides
	for addr, override := range overrides {
		if s.HasSelfDestructed(addr) {
			delete(overrides, addr)
			continue
		}
		copy := override.deepCopy()
		override.committed = copy.current
	}
	s.tempState = NewTemporaryState()
	s.tempState.storageOverrides = overrides
	s.journal = nil
	s.snapshottedCtxs = nil
	s.precompileErr = nil
	// Never flush: every cache layer belongs to this request and is discarded.
	s.Snapshot()
	return nil
}
