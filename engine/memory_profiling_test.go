package engine

import (
	"os"
	"testing"
	"time"
)

func TestMemoryProfiler(t *testing.T) {
	// Test creating a memory profiler
	profiler, err := NewMemoryProfiler(true, "")
	if err != nil {
		t.Fatalf("Failed to create memory profiler: %v", err)
	}
	defer profiler.Close()

	// Test starting profiling
	profiler.Start()

	// Test taking snapshots
	profiler.TakeSnapshot(1)
	
	// Wait a bit to avoid rate limiting
	time.Sleep(100 * time.Millisecond)
	profiler.TakeSnapshot(2)

	// Test getting snapshots
	snapshots := profiler.GetSnapshots()
	if len(snapshots) < 1 {
		t.Errorf("Expected at least 1 snapshot, got %d", len(snapshots))
	}

	// Test generating report
	report := profiler.GenerateReport()
	if !report.Enabled {
		t.Error("Report should show profiling as enabled")
	}
	if report.SnapshotCount < 1 {
		t.Errorf("Expected at least 1 snapshot in report, got %d", report.SnapshotCount)
	}

	// Test report string generation
	reportStr := report.String()
	if len(reportStr) == 0 {
		t.Error("Report string should not be empty")
	}
}

func TestMemoryProfilerWithLogFile(t *testing.T) {
	tempFile := "/tmp/test_memory_profile.jsonl"
	defer os.Remove(tempFile)

	profiler, err := NewMemoryProfiler(true, tempFile)
	if err != nil {
		t.Fatalf("Failed to create memory profiler with log file: %v", err)
	}
	defer profiler.Close()

	profiler.Start()
	profiler.TakeSnapshot(1)

	// Check that log file was created
	if _, err := os.Stat(tempFile); os.IsNotExist(err) {
		t.Error("Log file should have been created")
	}
}

func TestMemoryProfilerDisabled(t *testing.T) {
	profiler, err := NewMemoryProfiler(false, "")
	if err != nil {
		t.Fatalf("Failed to create disabled memory profiler: %v", err)
	}
	defer profiler.Close()

	profiler.Start()
	profiler.TakeSnapshot(1)

	snapshots := profiler.GetSnapshots()
	if len(snapshots) != 0 {
		t.Errorf("Disabled profiler should have no snapshots, got %d", len(snapshots))
	}

	report := profiler.GenerateReport()
	if report.Enabled {
		t.Error("Report should show profiling as disabled")
	}
}

func TestMemorySnapshotRateLimit(t *testing.T) {
	profiler, err := NewMemoryProfiler(true, "")
	if err != nil {
		t.Fatalf("Failed to create memory profiler: %v", err)
	}
	defer profiler.Close()

	profiler.Start()

	// Take multiple snapshots quickly
	profiler.TakeSnapshot(1)
	profiler.TakeSnapshot(2)
	profiler.TakeSnapshot(3)

	snapshots := profiler.GetSnapshots()
	// Should only have one snapshot due to rate limiting
	if len(snapshots) != 1 {
		t.Errorf("Expected 1 snapshot due to rate limiting, got %d", len(snapshots))
	}
}