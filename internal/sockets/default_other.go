//go:build !linux

package sockets

// Default returns the backend for this platform: lsof everywhere but Linux.
func Default() Backend {
	return &Lsof{}
}
