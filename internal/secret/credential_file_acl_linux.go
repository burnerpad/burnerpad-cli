//go:build linux

package secret

import "golang.org/x/sys/unix"

// Linux POSIX ACL access is bounded by the group-class mode bits checked by
// protectedUnixCredentialStat, so no independent ACL query is needed.
func credentialFileStat(fd int) (unix.Stat_t, bool, error) {
	var stat unix.Stat_t
	err := unix.Fstat(fd, &stat)
	return stat, false, err
}
