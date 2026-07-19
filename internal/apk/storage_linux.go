//go:build linux

package apk

import "golang.org/x/sys/unix"

func init() {
	publishNoReplace = publishNoReplaceLinux
}

func publishNoReplaceLinux(temp, dest string) error {
	return unix.Renameat2(unix.AT_FDCWD, temp, unix.AT_FDCWD, dest, unix.RENAME_NOREPLACE)
}
