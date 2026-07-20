package desktop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/yunsang/gitgit/internal/app"
	"github.com/yunsang/gitgit/internal/gitexec"
)

const (
	maximumCachedRepositories = 8
	maximumCachedHistoryPages = 16
	maximumCachedDetails      = 128
	maximumCachedBranches     = 1000
)

type repositoryCache struct {
	fingerprint string
	history     map[string]HistoryResponse
	historyKeys []string
	details     map[string]CommitDetail
	detailKeys  []string
	branches    map[string][]string
	branchKeys  []string
}

func newRepositoryCache() *repositoryCache {
	return &repositoryCache{
		history:  make(map[string]HistoryResponse),
		details:  make(map[string]CommitDetail),
		branches: make(map[string][]string),
	}
}

func repositoryFingerprint(ctx context.Context, repository *gitexec.Repository) (string, error) {
	refs, err := repository.Run(ctx, nil, "for-each-ref", "--format=%(refname)%00%(objectname)")
	if err != nil {
		return "", fmt.Errorf("fingerprint repository refs: %w", err)
	}
	digest := sha256.Sum256(refs)
	return hex.EncodeToString(digest[:]), nil
}

func historyCacheKey(request HistoryRequest, scope, related, head string, all bool, revisions []string) string {
	return fmt.Sprintf("branch-boundary-v2\x00%t\x00%s\x00%s\x00%s\x00%s\x00%d\x00%d", all, scope, related, head, strings.Join(revisions, "\x00"), request.Limit, request.Skip)
}

func detailCacheKey(oid, filePath string) string {
	return "first-parent-v1\x00" + oid + "\x00" + strings.TrimSpace(filePath)
}

func (s *Service) cachedHistory(root, fingerprint, key string) (HistoryResponse, bool) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	cache := s.cacheForRepositoryLocked(root)
	s.refreshRefCacheLocked(cache, fingerprint)
	response, ok := cache.history[key]
	if !ok && s.persistentCache != nil {
		if persisted, found, err := s.persistentCache.loadHistory(root, fingerprint, key); err == nil && found {
			response = persisted
			cache.history[key] = cloneHistoryResponse(persisted)
			cache.historyKeys = append(cache.historyKeys, key)
			trimCache(cache.history, &cache.historyKeys, maximumCachedHistoryPages)
			ok = true
		}
	}
	return cloneHistoryResponse(response), ok
}

func (s *Service) storeHistory(root, fingerprint, key string, response HistoryResponse) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	cache := s.cacheForRepositoryLocked(root)
	s.refreshRefCacheLocked(cache, fingerprint)
	if _, exists := cache.history[key]; !exists {
		cache.historyKeys = append(cache.historyKeys, key)
	}
	cache.history[key] = cloneHistoryResponse(response)
	trimCache(cache.history, &cache.historyKeys, maximumCachedHistoryPages)
	if s.persistentCache != nil {
		_ = s.persistentCache.storeHistory(root, fingerprint, key, response)
	}
}

func (s *Service) cachedDetail(root, key string) (CommitDetail, bool) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	cache := s.cacheForRepositoryLocked(root)
	detail, ok := cache.details[key]
	if !ok && s.persistentCache != nil {
		if persisted, found, err := s.persistentCache.loadDetail(root, key); err == nil && found {
			detail = persisted
			cache.details[key] = cloneCommitDetail(persisted)
			cache.detailKeys = append(cache.detailKeys, key)
			trimCache(cache.details, &cache.detailKeys, maximumCachedDetails)
			ok = true
		}
	}
	return cloneCommitDetail(detail), ok
}

func (s *Service) storeDetail(root, key string, detail CommitDetail) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	cache := s.cacheForRepositoryLocked(root)
	if _, exists := cache.details[key]; !exists {
		cache.detailKeys = append(cache.detailKeys, key)
	}
	cache.details[key] = cloneCommitDetail(detail)
	trimCache(cache.details, &cache.detailKeys, maximumCachedDetails)
	if s.persistentCache != nil {
		_ = s.persistentCache.storeDetail(root, key, detail)
	}
}

func (s *Service) cachedBranches(root, fingerprint, oid string) ([]string, bool) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	cache := s.cacheForRepositoryLocked(root)
	s.refreshRefCacheLocked(cache, fingerprint)
	branches, ok := cache.branches[oid]
	if !ok && s.persistentCache != nil {
		if persisted, found, err := s.persistentCache.loadBranches(root, fingerprint, oid); err == nil && found {
			branches = persisted
			cache.branches[oid] = append([]string(nil), persisted...)
			cache.branchKeys = append(cache.branchKeys, oid)
			trimCache(cache.branches, &cache.branchKeys, maximumCachedBranches)
			ok = true
		}
	}
	return append([]string(nil), branches...), ok
}

func (s *Service) storeBranches(root, fingerprint, oid string, branches []string) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	cache := s.cacheForRepositoryLocked(root)
	s.refreshRefCacheLocked(cache, fingerprint)
	if _, exists := cache.branches[oid]; !exists {
		cache.branchKeys = append(cache.branchKeys, oid)
	}
	cache.branches[oid] = append([]string(nil), branches...)
	trimCache(cache.branches, &cache.branchKeys, maximumCachedBranches)
	if s.persistentCache != nil {
		_ = s.persistentCache.storeBranches(root, fingerprint, oid, branches)
	}
}

func (s *Service) cacheForRepositoryLocked(root string) *repositoryCache {
	if s.caches == nil {
		s.caches = make(map[string]*repositoryCache)
	}
	if cache := s.caches[root]; cache != nil {
		return cache
	}
	if len(s.cacheRoots) >= maximumCachedRepositories {
		oldest := s.cacheRoots[0]
		delete(s.caches, oldest)
		s.cacheRoots = s.cacheRoots[1:]
	}
	cache := newRepositoryCache()
	s.caches[root] = cache
	s.cacheRoots = append(s.cacheRoots, root)
	return cache
}

func (s *Service) refreshRefCacheLocked(cache *repositoryCache, fingerprint string) {
	if cache.fingerprint == fingerprint {
		return
	}
	cache.fingerprint = fingerprint
	cache.history = make(map[string]HistoryResponse)
	cache.historyKeys = nil
	cache.branches = make(map[string][]string)
	cache.branchKeys = nil
}

func trimCache[T any](values map[string]T, keys *[]string, maximum int) {
	for len(*keys) > maximum {
		oldest := (*keys)[0]
		*keys = (*keys)[1:]
		delete(values, oldest)
	}
}

func cloneHistoryResponse(response HistoryResponse) HistoryResponse {
	cloned := response
	cloned.Branches = append([]string(nil), response.Branches...)
	cloned.Commits = make([]CommitSummary, len(response.Commits))
	for index, commit := range response.Commits {
		cloned.Commits[index] = cloneCommitSummary(commit)
	}
	return cloned
}

func cloneCommitDetail(detail CommitDetail) CommitDetail {
	detail.CommitSummary = cloneCommitSummary(detail.CommitSummary)
	return detail
}

func cloneCommitSummary(summary CommitSummary) CommitSummary {
	cloned := summary
	cloned.Parents = append([]string(nil), summary.Parents...)
	cloned.Refs = append([]string(nil), summary.Refs...)
	cloned.Branches = append([]string(nil), summary.Branches...)
	cloned.Files = append([]app.FileChange(nil), summary.Files...)
	return cloned
}
