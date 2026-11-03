package datastore

import "golang.org/x/sys/unix"

func publishDatabase(temporary, target string) error {
	return unix.RenamexNp(temporary, target, unix.RENAME_EXCL)
}
