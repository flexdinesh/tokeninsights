//go:build !linux && !darwin

package pipeline

// Without a supported stable filesystem identity, continue ordinary retry.
func rawSourceSignature(_ string) (string, error) { return "", nil }
