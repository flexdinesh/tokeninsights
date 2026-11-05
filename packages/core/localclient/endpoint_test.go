package localclient

import (
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"strings"
	"testing"
)

func TestDescriptorRejectsChangedProtocolAndSettings(t *testing.T) {
	options := testOptions(t)
	descriptor, _ := serveForTest(t, options)
	if err := descriptor.Compatible(descriptor.Settings); err != nil {
		t.Fatal(err)
	}
	changed := descriptor.Settings
	changed.CollectorDBPath += ".other"
	if !errors.Is(descriptor.Compatible(changed), localprotocol.ErrSettingsMismatch) {
		t.Fatal("different storage accepted")
	}
	descriptor.ProtocolVersion++
	err := descriptor.Compatible(descriptor.Settings)
	if !errors.Is(err, localprotocol.ErrIncompatible) || !strings.Contains(err.Error(), "backend protocol 4, client protocol 3") {
		t.Fatal("protocol mismatch lost", err)
	}
}
