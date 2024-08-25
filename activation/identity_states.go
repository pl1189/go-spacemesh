package activation

import (
	"errors"
	"maps"
	"slices"
	"sync"

	"github.com/spacemeshos/go-spacemesh/common/types"
)

var ErrInvalidIdentityStateSwitch = errors.New("invalid identity state switch")

type IdentityStateStorage struct {
	mu     sync.RWMutex
	states map[types.NodeID]types.IdentityState
}

func NewIdentityStateStorage() IdentityStates {
	return &IdentityStateStorage{
		states: make(map[types.NodeID]types.IdentityState),
	}
}

var validStateSwitch = map[types.IdentityState][]types.IdentityState{
	types.IdentityStateWaitForATXSyncing: {
		types.IdentityStateWaitForPoetRoundStart,
	},
	types.IdentityStatePostProving: {
		types.IdentityStateWaitForPoetRoundStart,
	},
	types.IdentityStateWaitForPoetRoundStart: {
		types.IdentityStateWaitForPoetRoundEnd,
		types.IdentityStateWaitForATXSyncing,
	},
	types.IdentityStateWaitForPoetRoundEnd: {
		types.IdentityStateFetchingProofs,
		types.IdentityStateWaitForPoetRoundStart,
	},
	types.IdentityStateFetchingProofs: {
		types.IdentityStatePostProving,
		types.IdentityStateWaitForPoetRoundStart,
	},
}

func (s *IdentityStateStorage) Set(id types.NodeID, newState types.IdentityState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	currentState, exists := s.states[id]
	switch {
	case !exists:
		if newState == types.IdentityStateWaitForATXSyncing {
			s.states[id] = newState
			return nil
		}
	case currentState == newState:
		return nil

	default:
		if validNextStates, ok := validStateSwitch[currentState]; ok &&
			slices.Contains(validNextStates, newState) {
			s.states[id] = newState
			return nil
		}
	}
	return ErrInvalidIdentityStateSwitch
}

func (s *IdentityStateStorage) Get(id types.NodeID) (types.IdentityState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, exists := s.states[id]
	if !exists {
		return 0, types.ErrIdentityStateUnknown
	}
	return state, nil
}

func (s *IdentityStateStorage) All() map[types.NodeID]types.IdentityState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copy := make(map[types.NodeID]types.IdentityState, len(s.states))
	maps.Copy(copy, s.states)
	return copy
}
