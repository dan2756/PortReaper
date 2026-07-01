package sockets

// Default returns the backend for this platform: /proc on Linux.
func Default() Backend {
	return &Proc{}
}
