//go:build !unix

package store

// lockPath is a no-op where flock does not exist (Windows development builds).
func lockPath(dir, path string) (func(), error) { return func() {}, nil }
