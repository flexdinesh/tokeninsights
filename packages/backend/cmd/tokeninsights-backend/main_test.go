package main

import (
	"strings"
	"testing"
)

func TestBackendRejectsEverythingExceptInternalServe(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"connect", "--json"},
		{"connect", "--json", "--launch", "tokeninsights-mygo://launch?id=0123456789abcdef0123456789abcdef"},
		{"internal-serve", "extra"},
		{"serve"},
	} {
		if err := run(t.Context(), args); err == nil || !strings.Contains(err.Error(), "usage:") {
			t.Fatal("unsupported action accepted", args, err)
		}
	}
}
