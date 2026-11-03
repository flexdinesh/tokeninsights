package datastore

import "golang.org/x/sys/unix"

func publishDatabase(temporary, target string) error {
	return unix.Renameat2(unix.AT_FDCWD, temporary, unix.AT_FDCWD, target, unix.RENAME_NOREPLACE)
}
