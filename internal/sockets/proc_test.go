package sockets

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeProc builds a procfs tree in a temp dir: the socket tables from
// testdata plus a few processes whose fds point at the sockets.
func fakeProc(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fds := func(pid int, links ...string) {
		t.Helper()
		dir := filepath.Join(root, fmt.Sprint(pid), "fd")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for i, l := range links {
			if err := os.Symlink(l, filepath.Join(dir, fmt.Sprint(i))); err != nil {
				t.Fatal(err)
			}
		}
	}
	tcp, err := os.ReadFile("testdata/proc_net_tcp")
	if err != nil {
		t.Fatal(err)
	}
	write("net/tcp", string(tcp)) // no tcp6: IPv6 disabled
	write("net/udp", "   sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops\n")
	write("self/status", "Name:\tportkill\n") // not a pid; must be skipped

	// node holds 127.0.0.1:3000 on two fds, plus some unrelated files.
	write("48213/status", "Name:\tnode\nPPid:\t48190\nUid:\t1000\t1000\t1000\t1000\n")
	write("48213/cmdline", "node\x00server.js\x00--port\x003000\x00")
	fds(48213, "/dev/null", "pipe:[99]", "socket:[418877]", "socket:[418877]", "socket:[419102]")

	// Two resolved processes sharing the same listening socket.
	for _, pid := range []int{900, 901} {
		write(fmt.Sprintf("%d/status", pid), "Name:\tsystemd-resolve\nPPid:\t1\nUid:\t101\t101\t101\t101\n")
		write(fmt.Sprintf("%d/cmdline", pid), "/lib/systemd/systemd-resolved\x00")
		fds(pid, "socket:[19873]")
	}

	// sshd's fd directory is missing, as if we weren't allowed to read it.
	write("812/status", "Name:\tsshd\nPPid:\t1\nUid:\t0\t0\t0\t0\n")
	return root
}

func TestProcBackend(t *testing.T) {
	p := &Proc{Root: fakeProc(t), Order: binary.LittleEndian}
	ls, err := p.Listeners(Options{UDP: true})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range ls {
		got = append(got, fmt.Sprintf("%s %s pid=%d ppid=%d uid=%d name=%s cmd=%q", l.Proto, l.Addr, l.PID, l.PPID, l.UID, l.Name, l.Command))
	}
	want := []string{
		`tcp 127.0.0.53:53 pid=900 ppid=1 uid=101 name=systemd-resolve cmd="/lib/systemd/systemd-resolved"`,
		`tcp 127.0.0.53:53 pid=901 ppid=1 uid=101 name=systemd-resolve cmd="/lib/systemd/systemd-resolved"`,
		`tcp 0.0.0.0:22 pid=0 ppid=0 uid=0 name= cmd=""`,
		`tcp 127.0.0.1:3000 pid=48213 ppid=48190 uid=1000 name=node cmd="node server.js --port 3000"`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, l := range ls {
		if l.UID == 0 && l.User != "root" {
			t.Errorf("uid 0 should resolve to root, got %q", l.User)
		}
	}
}

func TestProcBackendMissingTable(t *testing.T) {
	p := &Proc{Root: t.TempDir()}
	if _, err := p.Listeners(Options{}); err == nil {
		t.Fatal("expected an error without /proc/net/tcp")
	}
}
