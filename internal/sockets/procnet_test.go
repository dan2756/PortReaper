package sockets

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string, udp bool) []procSocket {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ss, err := parseProcNet(f, udp, binary.LittleEndian)
	if err != nil {
		t.Fatalf("parseProcNet(%s): %v", name, err)
	}
	return ss
}

func TestParseProcNetTCP(t *testing.T) {
	checkSockets(t, readFixture(t, "proc_net_tcp", false), []wantSocket{
		{"127.0.0.53:53", 101, 19873},
		{"0.0.0.0:22", 0, 22140},
		{"127.0.0.1:3000", 1000, 418877},
	})
}

func TestParseProcNetUDP(t *testing.T) {
	// Only unconnected sockets bound to a port count; the DHCP and DNS
	// client sockets are connected and the last one isn't bound yet.
	checkSockets(t, readFixture(t, "proc_net_udp", true), []wantSocket{
		{"127.0.0.53:53", 101, 19872},
		{"0.0.0.0:5353", 1000, 61220},
	})
}

func TestParseProcNetTCP6(t *testing.T) {
	checkSockets(t, readFixture(t, "proc_net_tcp6", false), []wantSocket{
		{"[::]:22", 0, 22142},
		{"[::1]:8080", 1000, 61002},
		{"[::ffff:127.0.0.1]:5173", 1000, 61177},
		{"[2001:db8::1]:443", 33, 70410},
		{"[fe80::5054:ff:fe12:3456]:9090", 1000, 70533},
	})
}

func TestParseProcNetUDP6(t *testing.T) {
	checkSockets(t, readFixture(t, "proc_net_udp6", true), []wantSocket{
		{"[::]:5353", 107, 21002},
		{"[fe80::5054:ff:fe12:3456]:546", 0, 19077},
	})
}

type wantSocket struct {
	addr  string
	uid   int
	inode uint64
}

func checkSockets(t *testing.T, got []procSocket, want []wantSocket) {
	t.Helper()
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
		if _, err := parseProcNet(strings.NewReader(header+row), false, binary.LittleEndian); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestParseHexAddrPortBigEndian(t *testing.T) {
	// On a big-endian host (s390x) the words come out in network order.
	for in, want := range map[string]string{
		"7F000001:0BB8":                         "127.0.0.1:3000",
		"00000000000000000000000000000001:1F90": "[::1]:8080",
		"20010DB8000000000000000000000001:01BB": "[2001:db8::1]:443",
	} {
		ap, err := parseHexAddrPort(in, binary.BigEndian)
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if ap.String() != want {
			t.Errorf("parseHexAddrPort(%s) = %s, want %s", in, ap, want)
		}
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
		// A process can't forge table rows or hide text with escapes.
		"python3\x00\n1      root  tcp  127.0.0.1:41211  /sbin/init\x00": `python3 \n1      root  tcp  127.0.0.1:41211  /sbin/init`,
		"sh\x00\x1b[8mhidden\rshown\x00":                                 `sh \x1b[8mhidden\rshown`,
	} {
		if got := parseCmdline([]byte(in)); got != want {
			t.Errorf("parseCmdline(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrintable(t *testing.T) {
	for in, want := range map[string]string{
		"node server.js --port 3000": "node server.js --port 3000",
		"a\tb\x7f\u009b":             `a\tb\x7f\u009b`,
		"evil\u202etxt.js":           `evil\u202etxt.js`,
		"bad\xffutf8":                `bad\xffutf8`,
		"日本語 café \\n":               "日本語 café \\n",
	} {
		if got := printable(in); got != want {
			t.Errorf("printable(%q) = %q, want %q", in, got, want)
		}
	}
}
