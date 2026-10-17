package hubupdate

import "golang.org/x/sys/unix"

// swap exchanges the two paths in one atomic rename, so neither is ever
// missing.
func swap(left, right string) error {
	return unix.RenamexNp(left, right, unix.RENAME_SWAP)
}
