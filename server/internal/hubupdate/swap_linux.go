package hubupdate

import "golang.org/x/sys/unix"

// swap exchanges the two paths in one atomic rename, as renamex_np does on
// macOS, so the tests run on Linux too.
func swap(left, right string) error {
	return unix.Renameat2(unix.AT_FDCWD, left, unix.AT_FDCWD, right, unix.RENAME_EXCHANGE)
}
