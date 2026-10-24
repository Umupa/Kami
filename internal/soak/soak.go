//go:build soak

// Package soak measures isolated long-running tests without user data or windows.
package soak

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func Rounds(t *testing.T, fallback int) int {
	t.Helper()
	if s := os.Getenv("KAMI_SOAK_ROUNDS"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 20 {
			t.Fatal("KAMI_SOAK_ROUNDS must be at least 20")
		}
		return n
	}
	return fallback
}

type Sample struct {
	Footprint  float64
	Heap       uint64
	Goroutines int
}

// Measure forces GC only in retention tests. Idle-recovery tests use natural GC.
func Measure(t *testing.T, round int) Sample {
	t.Helper()
	if os.Getenv("KAMI_SOAK_IDLE") == "" {
		runtime.GC()
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s := Sample{Heap: m.HeapAlloc, Goroutines: runtime.NumGoroutine()}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rss, err := exec.CommandContext(ctx, "/bin/ps", "-p", strconv.Itoa(os.Getpid()), "-o", "rss=").Output()
	if err != nil {
		t.Fatal(err)
	}
	vm, err := exec.CommandContext(ctx, "/usr/bin/vmmap", "-summary", strconv.Itoa(os.Getpid())).Output()
	if err != nil {
		t.Fatal(err)
	}
	var footprint string
	for _, line := range strings.Split(string(vm), "\n") {
		if strings.HasPrefix(line, "Physical footprint:") {
			footprint = strings.TrimSpace(strings.TrimPrefix(line, "Physical footprint:"))
		}
	}
	if footprint == "" {
		t.Fatal("vmmap did not report physical footprint")
	}
	unit := footprint[len(footprint)-1]
	value, err := strconv.ParseFloat(strings.TrimSpace(footprint[:len(footprint)-1]), 64)
	if err != nil {
		t.Fatal(err)
	}
	switch unit {
	case 'K':
		value /= 1024
	case 'M':
	case 'G':
		value *= 1024
	default:
		t.Fatalf("unexpected footprint unit: %s", footprint)
	}
	s.Footprint = value
	t.Logf("round=%d heap=%.2fMiB heapIdle=%.2fMiB heapReleased=%.2fMiB goroutines=%d rss=%sKiB footprint=%s", round, float64(s.Heap)/(1<<20), float64(m.HeapIdle)/(1<<20), float64(m.HeapReleased)/(1<<20), s.Goroutines, strings.TrimSpace(string(rss)), footprint)
	return s
}

func Check(t *testing.T, baseline, final Sample) {
	t.Helper()
	// Generous budgets catch regressions while allowing system cache variation.
	if final.Footprint > baseline.Footprint+64 {
		t.Errorf("physical footprint grew by %.1f MiB (budget 64 MiB)", final.Footprint-baseline.Footprint)
	}
	// Without a forced GC the heap includes garbage awaiting collection; one
	// unreclaimed 4K frame alone exceeds the budget.
	if os.Getenv("KAMI_SOAK_IDLE") == "" && final.Heap > baseline.Heap+32<<20 {
		t.Errorf("retained Go heap grew by more than 32 MiB")
	}
	if final.Goroutines > baseline.Goroutines+4 {
		t.Errorf("goroutines grew from %d to %d", baseline.Goroutines, final.Goroutines)
	}
}

func cpu(t *testing.T) time.Duration {
	t.Helper()
	var r syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &r); err != nil {
		t.Fatal(err)
	}
	return time.Duration(r.Utime.Sec+r.Stime.Sec)*time.Second + time.Duration(r.Utime.Usec+r.Stime.Usec)*time.Microsecond
}

// Idle measures CPU between samples, excluding resource-sampling subprocesses.
func Idle(t *testing.T) {
	t.Helper()
	duration := 5 * time.Second
	recovery := os.Getenv("KAMI_SOAK_IDLE")
	if recovery != "" {
		var err error
		duration, err = time.ParseDuration(recovery)
		if err != nil || duration < 30*time.Second || duration > 30*time.Minute {
			t.Fatal("KAMI_SOAK_IDLE must be between 30s and 30m")
		}
	}
	var first Sample
	if recovery != "" {
		first = Measure(t, 0)
	}
	remaining := duration
	for remaining > 0 {
		interval := min(30*time.Second, remaining)
		startCPU, start := cpu(t), time.Now()
		time.Sleep(interval)
		used, elapsed := cpu(t)-startCPU, time.Since(start)
		percent := 100 * float64(used) / float64(elapsed)
		remaining -= interval
		t.Logf("idle elapsed=%s CPU=%.3f%% of one core", duration-remaining, percent)
		if percent > 2 {
			t.Errorf("idle CPU exceeds 2%% of one core")
		}
		if recovery != "" {
			sample := Measure(t, 0)
			t.Logf("idle footprint change=%+.1fMiB", sample.Footprint-first.Footprint)
		}
	}
}
