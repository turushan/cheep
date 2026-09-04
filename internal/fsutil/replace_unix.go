//go:build !windows

package fsutil

import "os"

// Replace atomically moves source over destination on Unix-like systems.
func Replace(source string, destination string) error {
	return os.Rename(source, destination)
}
