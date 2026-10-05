//go:build !linux && !darwin

package serverstore

import "errors"

func publishDatabase(temporary, target string) error {
	return errors.New("atomic server database creation unsupported on this platform")
}
