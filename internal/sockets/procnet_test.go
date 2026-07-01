package sockets

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string) []procSocket {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ss, err := parseProcNet(f, binary.LittleEndian)
	if err != nil {
		t.Fatalf("parseProcNet(%s): %v", name, err)
	}
	return ss
}

func TestParseProcNetTCP(t *testing.T) {
	got := readFixture(t, "proc_net_tcp")
	want := []struct {
		addr  string
		uid   int
		inode uint64
	}{
		{"127.0.0.53:53", 101, 19873},
		{"0.0.0.0:22", 0, 22140},
		{"127.0.0.1:3000", 1000, 418877},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d listening sockets, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		g := got[i]
		if g.addr.String() != w.addr || g.uid != w.uid || g.inode != w.inode {
			t.Errorf("socket %d = {%s uid=%d inode=%d}, want {%s uid=%d inode=%d}",
				i, g.addr, g.uid, g.inode, w.addr, w.uid, w.inode)
		}
	}
}

func TestParseProcNetErrors(t *testing.T) {
	const header = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"
	for name, row := range map[string]string{
		"short row":   "   0: 0100007F:0BB8 00000000:0000 0A\n",
		"bad state":   "   0: 0100007F:0BB8 00000000:0000 ZZ 00000000:00000000 00:00000000 00000000  1000 0 1 1\n",
		"bad address": "   0: 0100007G:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000 0 1 1\n",
		"bad port":    "   0: 0100007F:XBB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000 0 1 1\n",
		"odd length":  "   0: 0100007F00:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000 0 1 1\n",
	} {
		if _, err := parseProcNet(strings.NewReader(header+row), binary.LittleEndian); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestParseHexAddrPortBigEndian(t *testing.T) {
	// On a big-endian host the kernel prints 127.0.0.1 as 7F000001.
	ap, err := parseHexAddrPort("7F000001:0BB8", binary.BigEndian)
	if err != nil {
		t.Fatal(err)
	}
	if ap.String() != "127.0.0.1:3000" {
		t.Errorf("got %s, want 127.0.0.1:3000", ap)
	}
}

func TestSocketInode(t *testing.T) {
	for link, want := range map[string]uint64{
		"socket:[418877]": 418877,
		"socket:[0]":      0,
	} {
		got, ok := socketInode(link)
		if !ok || got != want {
			t.Errorf("socketInode(%q) = %d, %v; want %d, true", link, got, ok, want)
		}
	}
	for _, link := range []string{"/dev/null", "pipe:[1234]", "socket:[12", "socket:[abc]", "anon_inode:[eventfd]"} {
		if _, ok := socketInode(link); ok {
			t.Errorf("socketInode(%q) should fail", link)
		}
	}
}

func TestParseStatus(t *testing.T) {
	f, err := os.Open("testdata/proc_status")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got := parseStatus(f)
	if got.name != "node" || got.ppid != 48190 || got.uid != 1000 {
		t.Errorf("parseStatus = %+v, want name=node ppid=48190 uid=1000", got)
	}
}

func TestParseCmdline(t *testing.T) {
	for in, want := range map[string]string{
		"node\x00/home/dj/app/node_modules/.bin/vite\x00--port\x003000\x00": "node /home/dj/app/node_modules/.bin/vite --port 3000",
		"/usr/sbin/sshd\x00-D\x00":      "/usr/sbin/sshd -D",
		"nginx: worker process\x00\x00": "nginx: worker process",
		"":                              "",
	} {
		if got := parseCmdline([]byte(in)); got != want {
			t.Errorf("parseCmdline(%q) = %q, want %q", in, got, want)
		}
	}
}
