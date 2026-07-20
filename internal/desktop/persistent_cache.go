package desktop

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/cockroachdb/pebble"
)

const (
	cacheBundleIdentifier = "com.wails.gitgit"
	cacheDirectoryVersion = "cache-v1"
)

// PersistentCache stores disposable repository metadata between app launches.
// Its contents are never authoritative and may be removed at any time.
type PersistentCache struct {
	mu sync.RWMutex
	db *pebble.DB
}

func DefaultPersistentCachePath() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate user cache directory: %w", err)
	}
	return filepath.Join(root, cacheBundleIdentifier, cacheDirectoryVersion), nil
}

func OpenDefaultPersistentCache() (*PersistentCache, error) {
	path, err := DefaultPersistentCachePath()
	if err != nil {
		return nil, err
	}
	return OpenPersistentCache(path)
}

func OpenPersistentCache(path string) (*PersistentCache, error) {
	path = filepath.Clean(path)
	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, fmt.Errorf("create persistent cache directory: %w", err)
	}
	database, err := pebble.Open(path, &pebble.Options{})
	if err != nil {
		return nil, fmt.Errorf("open persistent cache: %w", err)
	}
	return &PersistentCache{db: database}, nil
}

func (c *PersistentCache) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.db == nil {
		return nil
	}
	err := c.db.Close()
	c.db = nil
	return err
}

func (c *PersistentCache) loadHistory(repository, fingerprint, key string) (HistoryResponse, bool, error) {
	var response HistoryResponse
	found, err := c.loadRefValue(repository, fingerprint, persistentKey("history", repository, key), &response)
	return response, found, err
}

func (c *PersistentCache) storeHistory(repository, fingerprint, key string, response HistoryResponse) error {
	return c.storeRefValue(repository, fingerprint, persistentKey("history", repository, key), response)
}

func (c *PersistentCache) loadDetail(repository, key string) (CommitDetail, bool, error) {
	var detail CommitDetail
	found, err := c.loadValue(persistentKey("detail", repository, key), &detail)
	return detail, found, err
}

func (c *PersistentCache) storeDetail(repository, key string, detail CommitDetail) error {
	return c.storeValue(persistentKey("detail", repository, key), detail)
}

func (c *PersistentCache) loadBranches(repository, fingerprint, oid string) ([]string, bool, error) {
	var branches []string
	found, err := c.loadRefValue(repository, fingerprint, persistentKey("branches", repository, oid), &branches)
	return branches, found, err
}

func (c *PersistentCache) storeBranches(repository, fingerprint, oid string, branches []string) error {
	return c.storeRefValue(repository, fingerprint, persistentKey("branches", repository, oid), branches)
}

func (c *PersistentCache) loadRefValue(repository, fingerprint string, key []byte, target any) (bool, error) {
	if c == nil {
		return false, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureFingerprintLocked(repository, fingerprint); err != nil {
		return false, err
	}
	return c.loadValueLocked(key, target)
}

func (c *PersistentCache) storeRefValue(repository, fingerprint string, key []byte, value any) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureFingerprintLocked(repository, fingerprint); err != nil {
		return err
	}
	return c.storeValueLocked(key, value)
}

func (c *PersistentCache) loadValue(key []byte, target any) (bool, error) {
	if c == nil {
		return false, nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.loadValueLocked(key, target)
}

func (c *PersistentCache) storeValue(key []byte, value any) error {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.storeValueLocked(key, value)
}

func (c *PersistentCache) loadValueLocked(key []byte, target any) (bool, error) {
	if c.db == nil {
		return false, nil
	}
	value, closer, err := c.db.Get(key)
	if errors.Is(err, pebble.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer closer.Close()
	if err := json.Unmarshal(value, target); err != nil {
		return false, fmt.Errorf("decode persistent cache value: %w", err)
	}
	return true, nil
}

func (c *PersistentCache) storeValueLocked(key []byte, value any) error {
	if c.db == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode persistent cache value: %w", err)
	}
	if err := c.db.Set(key, encoded, pebble.NoSync); err != nil {
		return fmt.Errorf("write persistent cache value: %w", err)
	}
	return nil
}

func (c *PersistentCache) ensureFingerprintLocked(repository, fingerprint string) error {
	if c.db == nil {
		return nil
	}
	metadataKey := persistentMetadataKey(repository)
	stored, closer, err := c.db.Get(metadataKey)
	if err == nil {
		storedFingerprint := string(stored)
		closer.Close()
		if storedFingerprint == fingerprint {
			return nil
		}
	} else if !errors.Is(err, pebble.ErrNotFound) {
		return fmt.Errorf("read persistent cache fingerprint: %w", err)
	}

	batch := c.db.NewBatch()
	defer batch.Close()
	for _, namespace := range []string{"history", "branches"} {
		prefix := persistentRepositoryPrefix(namespace, repository)
		if err := batch.DeleteRange(prefix, prefixLimit(prefix), nil); err != nil {
			return fmt.Errorf("invalidate persistent %s cache: %w", namespace, err)
		}
	}
	if err := batch.Set(metadataKey, []byte(fingerprint), nil); err != nil {
		return fmt.Errorf("update persistent cache fingerprint: %w", err)
	}
	if err := batch.Commit(pebble.NoSync); err != nil {
		return fmt.Errorf("commit persistent cache invalidation: %w", err)
	}
	return nil
}

func persistentKey(namespace, repository, key string) []byte {
	prefix := persistentRepositoryPrefix(namespace, repository)
	encodedKey := base64.RawURLEncoding.EncodeToString([]byte(key))
	return append(prefix, encodedKey...)
}

func persistentRepositoryPrefix(namespace, repository string) []byte {
	return []byte("v1/" + namespace + "/" + repositoryCacheID(repository) + "/")
}

func persistentMetadataKey(repository string) []byte {
	return []byte("v1/meta/" + repositoryCacheID(repository) + "/refs")
}

func repositoryCacheID(repository string) string {
	digest := sha256.Sum256([]byte(filepath.Clean(repository)))
	return hex.EncodeToString(digest[:])
}

func prefixLimit(prefix []byte) []byte {
	limit := make([]byte, len(prefix)+1)
	copy(limit, prefix)
	limit[len(prefix)] = 0xff
	return limit
}
