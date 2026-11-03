package storagecontract

import (
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
)

// queryProfile excludes ingestion and fixture opening from CPU samples. Subtract
// heap-before from heap-after for query allocation samples; neither is peak RSS.
func queryProfile(b *testing.B) func() {
	b.Helper()
	profileDir := os.Getenv("TOKENINSIGHTS_QUERY_PROFILE_DIR")
	if profileDir == "" {
		return func() {}
	}
	directory := filepath.Join(profileDir, filepath.Base(os.Args[0]), strings.ReplaceAll(b.Name(), "/", "-"))
	if err := os.MkdirAll(filepath.Dir(directory), 0o700); err != nil {
		b.Fatal(err)
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		b.Fatal(err)
	}
	writeHeap := func(name string) {
		b.Helper()
		runtime.GC()
		f, err := os.Create(filepath.Join(directory, name+".pprof"))
		if err != nil {
			b.Fatal(err)
		}
		writeErr := pprof.WriteHeapProfile(f)
		closeErr := f.Close()
		if writeErr != nil || closeErr != nil {
			b.Fatal(writeErr, closeErr)
		}
	}
	writeHeap("heap-before")
	cpu, err := os.Create(filepath.Join(directory, "cpu.pprof"))
	if err != nil {
		b.Fatal(err)
	}
	if err := pprof.StartCPUProfile(cpu); err != nil {
		_ = cpu.Close()
		b.Fatal(err)
	}
	return func() {
		pprof.StopCPUProfile()
		closeErr := cpu.Close()
		if closeErr != nil {
			b.Fatal(closeErr)
		}
		writeHeap("heap-after")
	}
}
