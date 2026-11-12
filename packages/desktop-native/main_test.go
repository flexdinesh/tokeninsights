package main

import "testing"

func TestStandaloneArgumentsCannotSilentlySelectLocalData(t *testing.T) {
	for _, args := range [][]string{{"--connection", "ssh://host"}, {"tokeninsights-desktop://launch?id=0123456789abcdef0123456789abcdef"}} {
		if err := validateArguments(args); err == nil {
			t.Fatal("unsupported request fell through to standalone local capture", args)
		}
	}
	if err := validateArguments([]string{"tokeninsights-native://launch?id=0123456789abcdef0123456789abcdef"}); err != nil {
		t.Fatal(err)
	}
}
