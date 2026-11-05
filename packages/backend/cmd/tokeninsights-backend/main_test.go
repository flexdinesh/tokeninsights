package main

import (
	"strings"
	"testing"
)

func TestConnectorRejectsRetiredURLHandoffAndUnknownActions(t *testing.T) {
	for _, args := range [][]string{
		{"connect", "--json", "--url", "http://127.0.0.1:1234/"},
		{"connect", "--json", "--endpoint", "ssh://host"},
		{"serve"},
	} {
		if err := run(t.Context(), args); err == nil || !strings.Contains(err.Error(), "usage:") {
			t.Fatal("retired handoff accepted", args, err)
		}
	}
}
func TestExpiredPrivateHandoffNeverStartsLocalBackend(t *testing.T) {
	dashboard, handoff, err := connect(t.Context(), []string{"connect", "--json", "--launch", "tokeninsights-electron://launch?id=0123456789abcdef0123456789abcdef"})
	if err == nil || dashboard != nil || handoff != nil || !strings.Contains(err.Error(), "expired") {
		t.Fatal("expired handoff selected local defaults", err)
	}
}
