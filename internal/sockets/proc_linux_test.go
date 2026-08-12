//go:build linux

package sockets

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// These tests run the real /proc backend against sockets the test process
// opens itself. Nothing is killed.

func requireProcNet(t *testing.T, table string) {
	t.Helper()
	if _, err := os.Stat("/proc/net/" + table); err != nil {
		t.Skipf("no /proc/net/%s: %v", table, err)
	}
}

func findOwn(t *testing.T, opts Options, proto, addr string) Listener {
	t.Helper()
	ls, err := (&Proc{}).Listeners(opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range ls {
		if l.Addr == addr && l.Proto == proto && l.PID == os.Getpid() {
			return l
		}
	}
	t.Fatalf("%s %s owned by pid %d not found among %d listeners", proto, addr, os.Getpid(), len(ls))
	return Listener{}
}

func TestProcFindsOwnTCPListener(t *testing.T) {
	requireProcNet(t, "tcp")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	l := findOwn(t, Options{}, "tcp", "127.0.0.1:"+strconv.Itoa(port))
	if l.Port != port {
		t.Errorf("port = %d, want %d", l.Port, port)
	}
	if l.UID != os.Geteuid() {
		t.Errorf("uid = %d, want %d", l.UID, os.Geteuid())
	}
	if l.PPID != os.Getppid() {
		t.Errorf("ppid = %d, want %d", l.PPID, os.Getppid())
	}
	if exe := filepath.Base(os.Args[0]); !strings.Contains(l.Command, exe) {
		t.Errorf("command %q doesn't mention %s", l.Command, exe)
	}
}

func TestProcFindsOwnTCP6Listener(t *testing.T) {
	requireProcNet(t, "tcp6")
	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("no IPv6 loopback: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	findOwn(t, Options{}, "tcp6", "[::1]:"+strconv.Itoa(port))
}

func TestProcFindsOwnUDPSocket(t *testing.T) {
	requireProcNet(t, "udp")
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	port := pc.LocalAddr().(*net.UDPAddr).Port
	findOwn(t, Options{UDP: true}, "udp", "127.0.0.1:"+strconv.Itoa(port))

	// Without UDP the socket must not show up.
	ls, err := (&Proc{}).Listeners(Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range ls {
		if l.Port == port && strings.HasPrefix(l.Proto, "udp") {
			t.Errorf("UDP socket listed without Options.UDP: %+v", l)
		}
	}
}
