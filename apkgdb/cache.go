package apkgdb

import (
	"container/list"
	"log"
	"runtime"
	"sync"
	"time"
)

const (
	// cacheCleanupInterval is how often the cache cleanup runs.
	cacheCleanupInterval = 5 * time.Minute
	// cacheTTL is the time after which an unused entry is eligible for eviction.
	cacheTTL = 24 * time.Hour
	// cacheMemoryThreshold is the percentage of system memory at which eviction starts.
	// When memory usage exceeds this threshold, LRU eviction is triggered.
	cacheMemoryThreshold = 0.75
)

// cacheEntry wraps a Package with LRU and TTL tracking.
type cacheEntry struct {
	pkg        *Package
	hash       [32]byte
	lastAccess time.Time
	element    *list.Element // pointer to LRU list element
}

// packageCache implements an LRU cache with TTL-based eviction for packages.
// It monitors memory usage and evicts least recently used entries when
// memory pressure is high or entries haven't been accessed for 24 hours.
type packageCache struct {
	mu      sync.RWMutex
	entries map[[32]byte]*cacheEntry
	lru     *list.List // front = most recently used, back = least recently used
	stopCh  chan struct{}
}

// globalPkgCache is the singleton package cache instance.
var globalPkgCache = newPackageCache()

// newPackageCache creates and starts a new package cache.
func newPackageCache() *packageCache {
	c := &packageCache{
		entries: make(map[[32]byte]*cacheEntry),
		lru:     list.New(),
		stopCh:  make(chan struct{}),
	}
	go c.cleanupLoop()
	return c
}

// get retrieves a package from the cache by its hash.
// Returns nil if not found. Updates the last access time and LRU position.
func (c *packageCache) get(hash [32]byte) *Package {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[hash]
	if !ok {
		return nil
	}

	// Update last access time and move to front of LRU
	entry.lastAccess = time.Now()
	c.lru.MoveToFront(entry.element)

	return entry.pkg
}

// put adds a package to the cache. If the package already exists,
// it updates the last access time and LRU position.
func (c *packageCache) put(hash [32]byte, pkg *Package) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already exists
	if entry, ok := c.entries[hash]; ok {
		entry.lastAccess = time.Now()
		c.lru.MoveToFront(entry.element)
		return
	}

	// Create new entry
	entry := &cacheEntry{
		pkg:        pkg,
		hash:       hash,
		lastAccess: time.Now(),
	}
	entry.element = c.lru.PushFront(entry)
	c.entries[hash] = entry
}

// remove removes a package from the cache by its hash.
func (c *packageCache) remove(hash [32]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[hash]
	if !ok {
		return
	}

	c.lru.Remove(entry.element)
	delete(c.entries, hash)
}

// cleanupLoop periodically cleans up expired entries and checks memory pressure.
func (c *packageCache) cleanupLoop() {
	ticker := time.NewTicker(cacheCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCh:
			return
		}
	}
}

// cleanup removes expired entries and evicts LRU entries if memory pressure is high.
func (c *packageCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	expiredCount := 0
	memoryEvictCount := 0

	// First pass: remove entries that haven't been accessed in 24 hours
	for c.lru.Len() > 0 {
		elem := c.lru.Back()
		if elem == nil {
			break
		}

		entry := elem.Value.(*cacheEntry)
		if now.Sub(entry.lastAccess) < cacheTTL {
			// This and all entries towards the front are still valid
			break
		}

		// Entry has expired, remove it
		c.lru.Remove(elem)
		delete(c.entries, entry.hash)
		expiredCount++
	}

	// Second pass: if memory pressure is high, evict more LRU entries
	if isMemoryPressureHigh() {
		// Evict up to 25% of remaining entries
		targetEvict := c.lru.Len() / 4
		if targetEvict < 1 {
			targetEvict = 1
		}

		for i := 0; i < targetEvict && c.lru.Len() > 0; i++ {
			elem := c.lru.Back()
			if elem == nil {
				break
			}

			entry := elem.Value.(*cacheEntry)
			c.lru.Remove(elem)
			delete(c.entries, entry.hash)
			memoryEvictCount++
		}
	}

	if expiredCount > 0 || memoryEvictCount > 0 {
		log.Printf("apkgdb: cache cleanup: %d expired, %d memory-evicted, %d remaining",
			expiredCount, memoryEvictCount, c.lru.Len())
	}
}

// size returns the current number of entries in the cache.
func (c *packageCache) size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// stop stops the cache cleanup goroutine.
func (c *packageCache) stop() {
	close(c.stopCh)
}

// isMemoryPressureHigh checks if the system is under memory pressure.
// Returns true if allocated memory exceeds the threshold.
func isMemoryPressureHigh() bool {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Use HeapAlloc as a proxy for memory usage
	// Consider memory pressure high if we're using more than 1GB
	// or if the heap is growing significantly
	const maxHeapBytes = 1 << 30 // 1 GB

	if m.HeapAlloc > maxHeapBytes {
		return true
	}

	// Also check if we're using a large percentage of system memory
	// HeapSys is the total heap memory obtained from the OS
	if m.HeapSys > 0 && float64(m.HeapAlloc)/float64(m.HeapSys) > cacheMemoryThreshold {
		return true
	}

	return false
}
