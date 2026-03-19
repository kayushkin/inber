// memory_profiling.go provides memory usage monitoring for long sessions.
package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
)

// MemorySnapshot captures memory usage at a specific point in time.
type MemorySnapshot struct {
	Timestamp       time.Time `json:"timestamp"`
	TurnNumber      int       `json:"turn_number"`
	HeapAllocMB     float64   `json:"heap_alloc_mb"`
	HeapSysMB       float64   `json:"heap_sys_mb"`
	HeapIdleMB      float64   `json:"heap_idle_mb"`
	HeapInuseMB     float64   `json:"heap_inuse_mb"`
	HeapReleasedMB  float64   `json:"heap_released_mb"`
	NumGC           uint32    `json:"num_gc"`
	GCCPUFraction   float64   `json:"gc_cpu_fraction"`
	NumGoroutine    int       `json:"num_goroutine"`
	StackInuseMB    float64   `json:"stack_inuse_mb"`
	MSpanInuseMB    float64   `json:"mspan_inuse_mb"`
	MCacheInuseMB   float64   `json:"mcache_inuse_mb"`
	TotalAllocMB    float64   `json:"total_alloc_mb"`
	Mallocs         uint64    `json:"mallocs"`
	Frees           uint64    `json:"frees"`
}

// MemoryProfiler tracks memory usage during engine execution.
type MemoryProfiler struct {
	mu          sync.RWMutex
	enabled     bool
	snapshots   []MemorySnapshot
	startTime   time.Time
	logFile     *os.File
	encoder     *json.Encoder
	lastSnapshot time.Time
	interval    time.Duration
}

// NewMemoryProfiler creates a new memory profiler.
func NewMemoryProfiler(enabled bool, logPath string) (*MemoryProfiler, error) {
	mp := &MemoryProfiler{
		enabled:  enabled,
		interval: 30 * time.Second, // snapshot every 30 seconds by default
	}

	if enabled && logPath != "" {
		// Create log file for memory snapshots
		f, err := os.Create(logPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create memory profile log: %w", err)
		}
		mp.logFile = f
		mp.encoder = json.NewEncoder(f)
	}

	return mp, nil
}

// Start begins memory profiling.
func (mp *MemoryProfiler) Start() {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if !mp.enabled {
		return
	}

	mp.startTime = time.Now()
	mp.lastSnapshot = time.Time{}
	mp.snapshots = nil
}

// TakeSnapshot captures current memory statistics.
func (mp *MemoryProfiler) TakeSnapshot(turnNumber int) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if !mp.enabled {
		return
	}

	now := time.Now()
	
	// Rate limiting: don't take snapshots too frequently
	if !mp.lastSnapshot.IsZero() && now.Sub(mp.lastSnapshot) < mp.interval {
		return
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	snapshot := MemorySnapshot{
		Timestamp:      now,
		TurnNumber:     turnNumber,
		HeapAllocMB:    bytesToMB(m.HeapAlloc),
		HeapSysMB:      bytesToMB(m.HeapSys),
		HeapIdleMB:     bytesToMB(m.HeapIdle),
		HeapInuseMB:    bytesToMB(m.HeapInuse),
		HeapReleasedMB: bytesToMB(m.HeapReleased),
		NumGC:          m.NumGC,
		GCCPUFraction:  m.GCCPUFraction,
		NumGoroutine:   runtime.NumGoroutine(),
		StackInuseMB:   bytesToMB(m.StackInuse),
		MSpanInuseMB:   bytesToMB(m.MSpanInuse),
		MCacheInuseMB:  bytesToMB(m.MCacheInuse),
		TotalAllocMB:   bytesToMB(m.TotalAlloc),
		Mallocs:        m.Mallocs,
		Frees:          m.Frees,
	}

	mp.snapshots = append(mp.snapshots, snapshot)
	mp.lastSnapshot = now

	// Write to log file if available
	if mp.encoder != nil {
		mp.encoder.Encode(snapshot)
	}
}

// GetSnapshots returns all collected memory snapshots.
func (mp *MemoryProfiler) GetSnapshots() []MemorySnapshot {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	// Return a copy to avoid race conditions
	snapshots := make([]MemorySnapshot, len(mp.snapshots))
	copy(snapshots, mp.snapshots)
	return snapshots
}

// Close cleans up the profiler resources.
func (mp *MemoryProfiler) Close() error {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if mp.logFile != nil {
		return mp.logFile.Close()
	}
	return nil
}

// GenerateReport creates a summary of memory usage patterns.
func (mp *MemoryProfiler) GenerateReport() MemoryReport {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	if len(mp.snapshots) == 0 {
		return MemoryReport{
			Enabled: mp.enabled,
			Duration: time.Since(mp.startTime),
		}
	}

	first := mp.snapshots[0]
	last := mp.snapshots[len(mp.snapshots)-1]

	var maxHeap, minHeap float64
	var maxGoroutines, minGoroutines int
	var totalGCs uint32

	maxHeap = first.HeapAllocMB
	minHeap = first.HeapAllocMB
	maxGoroutines = first.NumGoroutine
	minGoroutines = first.NumGoroutine

	for _, snap := range mp.snapshots {
		if snap.HeapAllocMB > maxHeap {
			maxHeap = snap.HeapAllocMB
		}
		if snap.HeapAllocMB < minHeap {
			minHeap = snap.HeapAllocMB
		}
		if snap.NumGoroutine > maxGoroutines {
			maxGoroutines = snap.NumGoroutine
		}
		if snap.NumGoroutine < minGoroutines {
			minGoroutines = snap.NumGoroutine
		}
	}

	totalGCs = last.NumGC - first.NumGC

	return MemoryReport{
		Enabled:           mp.enabled,
		Duration:          time.Since(mp.startTime),
		SnapshotCount:     len(mp.snapshots),
		StartHeapMB:       first.HeapAllocMB,
		EndHeapMB:         last.HeapAllocMB,
		MaxHeapMB:         maxHeap,
		MinHeapMB:         minHeap,
		HeapGrowthMB:      last.HeapAllocMB - first.HeapAllocMB,
		StartGoroutines:   first.NumGoroutine,
		EndGoroutines:     last.NumGoroutine,
		MaxGoroutines:     maxGoroutines,
		MinGoroutines:     minGoroutines,
		GoroutineGrowth:   last.NumGoroutine - first.NumGoroutine,
		GCCount:           totalGCs,
		AvgGCCPUFraction:  averageGCCPUFraction(mp.snapshots),
		TotalAllocGrowthMB: last.TotalAllocMB - first.TotalAllocMB,
	}
}

// MemoryReport summarizes memory usage over the profiled period.
type MemoryReport struct {
	Enabled             bool          `json:"enabled"`
	Duration            time.Duration `json:"duration"`
	SnapshotCount       int           `json:"snapshot_count"`
	StartHeapMB         float64       `json:"start_heap_mb"`
	EndHeapMB           float64       `json:"end_heap_mb"`
	MaxHeapMB           float64       `json:"max_heap_mb"`
	MinHeapMB           float64       `json:"min_heap_mb"`
	HeapGrowthMB        float64       `json:"heap_growth_mb"`
	StartGoroutines     int           `json:"start_goroutines"`
	EndGoroutines       int           `json:"end_goroutines"`
	MaxGoroutines       int           `json:"max_goroutines"`
	MinGoroutines       int           `json:"min_goroutines"`
	GoroutineGrowth     int           `json:"goroutine_growth"`
	GCCount             uint32        `json:"gc_count"`
	AvgGCCPUFraction    float64       `json:"avg_gc_cpu_fraction"`
	TotalAllocGrowthMB  float64       `json:"total_alloc_growth_mb"`
}

// String returns a human-readable summary of the memory report.
func (r MemoryReport) String() string {
	if !r.Enabled {
		return "Memory profiling: disabled"
	}

	return fmt.Sprintf(`Memory Profile Report:
Duration: %v
Snapshots: %d
Heap: %.1fMB → %.1fMB (growth: %.1fMB, max: %.1fMB)
Goroutines: %d → %d (growth: %d, max: %d)
GC: %d cycles, avg CPU: %.2f%%
Total Alloc Growth: %.1fMB`,
		r.Duration.Round(time.Second),
		r.SnapshotCount,
		r.StartHeapMB, r.EndHeapMB, r.HeapGrowthMB, r.MaxHeapMB,
		r.StartGoroutines, r.EndGoroutines, r.GoroutineGrowth, r.MaxGoroutines,
		r.GCCount, r.AvgGCCPUFraction*100,
		r.TotalAllocGrowthMB)
}

// Helper functions

func bytesToMB(bytes uint64) float64 {
	return float64(bytes) / (1024 * 1024)
}

func averageGCCPUFraction(snapshots []MemorySnapshot) float64 {
	if len(snapshots) == 0 {
		return 0
	}
	
	var sum float64
	for _, snap := range snapshots {
		sum += snap.GCCPUFraction
	}
	return sum / float64(len(snapshots))
}