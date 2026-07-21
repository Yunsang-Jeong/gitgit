package desktop

import (
	"context"

	"github.com/yunsang/gitgit/internal/gitexec"
)

func (s *Service) beginOperation(parent context.Context) (context.Context, func()) {
	return s.beginOperationWithKey(parent, "")
}

// beginLatestOperation keeps at most one active operation for a read surface.
// Starting a newer request cancels the Git subprocesses owned by the older
// request instead of merely discarding its response in the frontend.
func (s *Service) beginLatestOperation(parent context.Context, key string) (context.Context, func()) {
	return s.beginOperationWithKey(parent, key)
}

func (s *Service) beginOperationWithKey(parent context.Context, key string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	if s.operations == nil {
		s.operations = make(map[uint64]context.CancelFunc)
	}
	if s.latestOperations == nil {
		s.latestOperations = make(map[string]uint64)
	}
	var previousCancel context.CancelFunc
	if key != "" {
		if previousID, ok := s.latestOperations[key]; ok {
			previousCancel = s.operations[previousID]
			delete(s.operations, previousID)
		}
	}
	s.nextOperationID++
	id := s.nextOperationID
	s.operations[id] = cancel
	if key != "" {
		s.latestOperations[key] = id
	}
	s.mu.Unlock()
	if previousCancel != nil {
		previousCancel()
	}

	return ctx, func() {
		cancel()
		s.mu.Lock()
		delete(s.operations, id)
		if key != "" && s.latestOperations[key] == id {
			delete(s.latestOperations, key)
		}
		s.mu.Unlock()
	}
}

func (s *Service) beginRepositorySwitch(parent context.Context) (context.Context, uint64, func()) {
	generation := s.reserveRepositorySwitch()
	ctx, finish, _ := s.beginReservedRepositorySwitch(parent, generation)
	return ctx, generation, finish
}

func (s *Service) reserveRepositorySwitch() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.repositoryGeneration++
	return s.repositoryGeneration
}

func (s *Service) beginReservedRepositorySwitch(parent context.Context, generation uint64) (context.Context, func(), bool) {
	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	if generation != s.repositoryGeneration {
		s.mu.Unlock()
		cancel()
		return ctx, func() {}, false
	}
	cancels := make([]context.CancelFunc, 0, len(s.operations)+1)
	for _, operationCancel := range s.operations {
		cancels = append(cancels, operationCancel)
	}
	if s.searchCancel != nil {
		cancels = append(cancels, s.searchCancel)
		s.searchCancel = nil
	}
	s.operations = make(map[uint64]context.CancelFunc)
	s.latestOperations = make(map[string]uint64)
	s.searchID++
	s.switchingRepository = true
	s.nextOperationID++
	id := s.nextOperationID
	s.operations[id] = cancel
	s.mu.Unlock()

	for _, operationCancel := range cancels {
		operationCancel()
	}

	return ctx, func() {
		cancel()
		s.mu.Lock()
		delete(s.operations, id)
		s.mu.Unlock()
	}, true
}

func (s *Service) completeRepositorySwitch(generation uint64, repository *gitexec.Repository) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.repositoryGeneration {
		return false
	}
	s.repository = repository
	s.switchingRepository = false
	return true
}

func (s *Service) failRepositorySwitch(generation uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation == s.repositoryGeneration {
		s.switchingRepository = false
	}
}

func (s *Service) CancelOperations() {
	s.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.operations)+1)
	for _, cancel := range s.operations {
		cancels = append(cancels, cancel)
	}
	if s.searchCancel != nil {
		cancels = append(cancels, s.searchCancel)
		s.searchCancel = nil
	}
	s.operations = make(map[uint64]context.CancelFunc)
	s.latestOperations = make(map[string]uint64)
	s.searchID++
	s.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}
