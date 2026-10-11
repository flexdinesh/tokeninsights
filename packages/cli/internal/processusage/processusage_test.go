package processusage

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"
)

type fixture struct {
	sampler     *Sampler
	time        time.Time
	cpu         float64
	memory      uint64
	cpuErr      error
	memoryErr   error
	cpuReads    int
	memoryReads int
}

func newFixture() *fixture {
	f := &fixture{time: time.Unix(100, 0), cpu: 4, memory: 1024}
	f.sampler = &Sampler{
		now: func() time.Time { return f.time },
		readCPU: func(context.Context) (float64, error) {
			f.cpuReads++
			return f.cpu, f.cpuErr
		},
		readMemory: func(context.Context) (uint64, error) {
			f.memoryReads++
			return f.memory, f.memoryErr
		},
	}
	return f
}

func TestCPUTracksElapsedProcessTime(t *testing.T) {
	f := newFixture()
	first := f.sampler.Snapshot(t.Context())
	if first.CPUPercent != nil {
		t.Fatalf("initial CPU must be unknown, got %v", *first.CPUPercent)
	}
	assertMemory(t, first, int64(f.memory))
	f.time = f.time.Add(2 * time.Second)
	f.cpu += 5
	assertCPU(t, f.sampler.Snapshot(t.Context()), 250)
	f.time = f.time.Add(time.Second)
	assertCPU(t, f.sampler.Snapshot(t.Context()), 0)
}

func TestPollsShareCacheWithoutMovingBaseline(t *testing.T) {
	f := newFixture()
	first := f.sampler.Snapshot(t.Context())
	*first.ResidentMemoryBytes = 0
	f.time = f.time.Add(time.Second / 2)
	f.cpu += 1
	f.memory *= 2
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			assertMemory(t, f.sampler.Snapshot(t.Context()), 1024)
		})
	}
	wg.Wait()
	if f.cpuReads != 1 || f.memoryReads != 1 {
		t.Fatalf("cached reads: CPU %d, memory %d", f.cpuReads, f.memoryReads)
	}
	f.time = f.time.Add(time.Second / 2)
	assertCPU(t, f.sampler.Snapshot(t.Context()), 100)
	assertMemory(t, f.sampler.Snapshot(t.Context()), 2048)
}

func TestIndependentErrorsAndRecovery(t *testing.T) {
	f := newFixture()
	f.sampler.Snapshot(t.Context())
	f.time = f.time.Add(time.Second)
	f.cpuErr = errors.New("unavailable CPU")
	f.cpu += 1
	sample := f.sampler.Snapshot(t.Context())
	if sample.CPUPercent != nil {
		t.Fatalf("failed CPU must be unknown, got %v", *sample.CPUPercent)
	}
	assertMemory(t, sample, int64(f.memory))
	f.time = f.time.Add(time.Second)
	f.cpuErr = nil
	f.cpu += 1
	f.memoryErr = errors.New("unavailable RSS")
	sample = f.sampler.Snapshot(t.Context())
	assertCPU(t, sample, 100)
	if sample.ResidentMemoryBytes != nil {
		t.Fatalf("failed RSS must be unknown, got %v", *sample.ResidentMemoryBytes)
	}
	f.time = f.time.Add(time.Second)
	f.memoryErr = nil
	assertMemory(t, f.sampler.Snapshot(t.Context()), int64(f.memory))
}

func TestCanceledReadsDoNotReplaceCacheOrBaseline(t *testing.T) {
	f := newFixture()
	f.sampler.Snapshot(t.Context())
	f.time = f.time.Add(time.Second)
	f.cpu += 1
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if sample := f.sampler.Snapshot(ctx); sample.CPUPercent != nil || sample.ResidentMemoryBytes != nil {
		t.Fatalf("canceled snapshot: %+v", sample)
	}
	if f.cpuReads != 1 || f.memoryReads != 1 {
		t.Fatalf("canceled request read process: CPU %d, memory %d", f.cpuReads, f.memoryReads)
	}
	ctx, cancel = context.WithCancel(t.Context())
	readCPU := f.sampler.readCPU
	f.sampler.readCPU = func(ctx context.Context) (float64, error) {
		cancel()
		return readCPU(ctx)
	}
	if sample := f.sampler.Snapshot(ctx); sample.CPUPercent != nil || sample.ResidentMemoryBytes != nil {
		t.Fatalf("mid-read cancellation: %+v", sample)
	}
	f.sampler.readCPU = readCPU
	f.time = f.time.Add(time.Second)
	f.cpu += 1
	assertCPU(t, f.sampler.Snapshot(t.Context()), 100)
	if f.cpuReads != 3 || f.memoryReads != 3 {
		t.Fatalf("live retry was cached: CPU %d, memory %d", f.cpuReads, f.memoryReads)
	}
}

func TestUnavailableInitialCPURecoversWithBaseline(t *testing.T) {
	f := newFixture()
	f.cpuErr = errors.New("unavailable CPU")
	f.sampler.Snapshot(t.Context())
	f.time = f.time.Add(time.Second)
	f.cpuErr = nil
	if sample := f.sampler.Snapshot(t.Context()); sample.CPUPercent != nil {
		t.Fatalf("first successful CPU read must establish baseline, got %v", *sample.CPUPercent)
	}
	f.time = f.time.Add(time.Second)
	f.cpu++
	assertCPU(t, f.sampler.Snapshot(t.Context()), 100)
}

func TestInvalidMeasurementsRemainUnknown(t *testing.T) {
	for name, cpu := range map[string]float64{
		"negative": -1,
		"NaN":      math.NaN(),
		"infinite": math.Inf(1),
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			f.sampler.Snapshot(t.Context())
			f.time = f.time.Add(time.Second)
			f.cpu = cpu
			if sample := f.sampler.Snapshot(t.Context()); sample.CPUPercent != nil {
				t.Fatalf("invalid CPU leaked: %v", *sample.CPUPercent)
			}
		})
	}
	f := newFixture()
	f.memory = math.MaxInt64 + 1
	if sample := f.sampler.Snapshot(t.Context()); sample.ResidentMemoryBytes != nil {
		t.Fatalf("overflowing RSS leaked: %v", *sample.ResidentMemoryBytes)
	}
}

func TestCPUCounterResetEstablishesNewBaseline(t *testing.T) {
	f := newFixture()
	f.sampler.Snapshot(t.Context())
	f.time = f.time.Add(time.Second)
	f.cpu = 1
	if sample := f.sampler.Snapshot(t.Context()); sample.CPUPercent != nil {
		t.Fatalf("reset CPU must be unknown, got %v", *sample.CPUPercent)
	}
	f.time = f.time.Add(time.Second)
	f.cpu++
	assertCPU(t, f.sampler.Snapshot(t.Context()), 100)
}

func TestNativeResidentMemory(t *testing.T) {
	sample := New().Snapshot(t.Context())
	if sample.ResidentMemoryBytes == nil || *sample.ResidentMemoryBytes <= 0 {
		t.Fatalf("native resident memory unavailable: %+v", sample)
	}
	if sample.CPUPercent != nil {
		t.Fatalf("native initial CPU must be unknown: %v", *sample.CPUPercent)
	}
}

func assertCPU(t *testing.T, sample Sample, want float64) {
	t.Helper()
	if sample.CPUPercent == nil {
		t.Fatalf("CPU unavailable, want %v", want)
	}
	if *sample.CPUPercent != want {
		t.Fatalf("CPU %v, want %v", *sample.CPUPercent, want)
	}
}

func assertMemory(t *testing.T, sample Sample, want int64) {
	t.Helper()
	if sample.ResidentMemoryBytes == nil {
		t.Fatalf("resident memory unavailable, want %v", want)
	}
	if *sample.ResidentMemoryBytes != want {
		t.Fatalf("resident memory %v, want %v", *sample.ResidentMemoryBytes, want)
	}
}
