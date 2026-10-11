// Package processusage samples CPU time and resident memory for this process.
package processusage

import (
	"context"
	"math"
	"os"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

const (
	minimumInterval = time.Second
	percentScale    = 100
)

// Sample reports unavailable measurements as nil. CPUPercent measures process
// CPU time against elapsed wall time: one busy core is 100 percent.
type Sample struct {
	CPUPercent          *float64
	ResidentMemoryBytes *int64
}

// Sampler serializes reads and shares a sample for at least one second. It owns
// no workers or handles; CPU needs two successful reads before it is available.
type Sampler struct {
	mu         sync.Mutex
	now        func() time.Time
	readCPU    func(context.Context) (float64, error)
	readMemory func(context.Context) (uint64, error)
	lastSample time.Time
	cached     Sample
	baseline   time.Time
	cpuSeconds float64
}

// New binds the sampler to this process, without enumerating other processes or
// reading arguments, environment variables or other process metadata.
func New() *Sampler {
	current := &process.Process{Pid: int32(os.Getpid())}
	return &Sampler{
		now: time.Now,
		readCPU: func(ctx context.Context) (float64, error) {
			times, err := current.TimesWithContext(ctx)
			if err != nil {
				return 0, err
			}
			return times.User + times.System, nil
		},
		readMemory: func(ctx context.Context) (uint64, error) {
			memory, err := current.MemoryInfoWithContext(ctx)
			if err != nil {
				return 0, err
			}
			return memory.RSS, nil
		},
	}
}

// Snapshot reads at most once per interval. Failed measurements do not hide a
// successful independent measurement. Cancellation never replaces the cache or
// CPU baseline; callers can retry with a live context.
func (s *Sampler) Snapshot(ctx context.Context) Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return Sample{}
	}
	now := s.now()
	if !s.lastSample.IsZero() && now.Sub(s.lastSample) < minimumInterval {
		return s.cached.copy()
	}
	cpuSeconds, cpuErr := s.readCPU(ctx)
	memoryBytes, memoryErr := s.readMemory(ctx)
	if ctx.Err() != nil {
		return Sample{}
	}
	sample := Sample{}
	if cpuErr == nil && validCPU(cpuSeconds) {
		elapsed := now.Sub(s.baseline).Seconds()
		if !s.baseline.IsZero() && elapsed > 0 && cpuSeconds >= s.cpuSeconds {
			percent := (cpuSeconds - s.cpuSeconds) / elapsed * percentScale
			if validCPU(percent) {
				sample.CPUPercent = &percent
			}
		}
		s.baseline = now
		s.cpuSeconds = cpuSeconds
	}
	if memoryErr == nil && memoryBytes <= math.MaxInt64 {
		resident := int64(memoryBytes)
		sample.ResidentMemoryBytes = &resident
	}
	s.lastSample = now
	s.cached = sample
	return sample.copy()
}

func validCPU(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (s Sample) copy() Sample {
	if s.CPUPercent != nil {
		value := *s.CPUPercent
		s.CPUPercent = &value
	}
	if s.ResidentMemoryBytes != nil {
		value := *s.ResidentMemoryBytes
		s.ResidentMemoryBytes = &value
	}
	return s
}
