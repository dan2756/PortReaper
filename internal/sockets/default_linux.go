package sockets

import "os/exec"

// Default returns the backend for this platform: /proc on Linux, falling
// back to lsof when /proc shows nothing and lsof is installed.
func Default() Backend {
	b := &Fallback{Primary: &Proc{}}
	if path, err := exec.LookPath("lsof"); err == nil {
		b.Secondary = &Lsof{Path: path}
	}
	return b
}
