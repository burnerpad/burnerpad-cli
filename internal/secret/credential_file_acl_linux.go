//go:build linux

package secret

// Linux POSIX ACL access is bounded by the group-class mode bits checked by
// protectedUnixCredentialStat, so no independent ACL query is needed.
func credentialFileHasExtendedACL(int) (bool, error) { return false, nil }
