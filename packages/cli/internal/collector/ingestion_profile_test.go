package collector_test

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
)

var ingestionProfileDir = flag.String("ingestion-profile-dir", "", "write ingestion profiles around measured work only; use -benchtime=1x -count=1")

func ingestionProfile(b *testing.B) func() {
	b.Helper()
	if *ingestionProfileDir == "" {
		return func() {}
	}
	if b.N != 1 {
		b.Fatal("ingestion profiles require -benchtime=1x")
	}
	directory := filepath.Join(*ingestionProfileDir, strings.ReplaceAll(b.Name(), "/", "-"))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		b.Fatal(err)
	}
	write := func(name, file string) {
		b.Helper()
		output, err := os.Create(filepath.Join(directory, file+".pprof"))
		if err != nil {
			b.Fatal(err)
		}
		writeErr := pprof.Lookup(name).WriteTo(output, 0)
		closeErr := output.Close()
		if writeErr != nil || closeErr != nil {
			b.Fatal(writeErr, closeErr)
		}
	}
	runtime.GC()
	write("heap", "heap-before")
	write("mutex", "mutex-before")
	write("block", "block-before")
	previousMutexRate := runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)
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
		if err := cpu.Close(); err != nil {
			b.Fatal(err)
		}
		runtime.SetMutexProfileFraction(previousMutexRate)
		runtime.SetBlockProfileRate(0)
		runtime.GC()
		write("heap", "heap-after")
		write("mutex", "mutex-after")
		write("block", "block-after")
	}
}
