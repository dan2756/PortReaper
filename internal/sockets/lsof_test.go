package sockets

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func parseLsofFixture(t *testing.T, name, proto string) []Listener {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ls, err := parseLsof(f, proto)
	if err != nil {
		t.Fatalf("parseLsof(%s): %v", name, err)
	}
	return ls
}

// row renders the fields the parser is responsible for.
func row(l Listener) string {
	return fmt.Sprintf("%d/%d %s(%d) %s %s %s %d", l.PID, l.PPID, l.User, l.UID, l.Name, l.Proto, l.Addr, l.Port)
}

func checkRows(t *testing.T, got []Listener, want []string) {
	t.Helper()
	var rows []string
	for _, l := range got {
		rows = append(rows, row(l))
	}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
}

func TestParseLsofDarwinTCP(t *testing.T) {
	// macOS lsof prints an 'f' line before every file.
	checkRows(t, parseLsofFixture(t, "lsof_tcp_darwin.txt", "tcp"), []string{
		"512/1 dj(501) ControlCe tcp *:7000 7000",
		"512/1 dj(501) ControlCe tcp *:7000 7000",
		"512/1 dj(501) ControlCe tcp *:5000 5000",
		"512/1 dj(501) ControlCe tcp *:5000 5000",
		"48213/48190 dj(501) node tcp 127.0.0.1:3000 3000",
		"48213/48190 dj(501) node tcp6 [::1]:3000 3000",
		"1104/1 _postgres(502) postgres tcp6 [::1]:5432 5432",
		"1104/1 _postgres(502) postgres tcp 127.0.0.1:5432 5432",
	})
}

func TestParseLsofDarwinUDP(t *testing.T) {
	// The wildcard "*:*" socket and the connected one are not listening.
	checkRows(t, parseLsofFixture(t, "lsof_udp_darwin.txt", "udp"), []string{
		"312/1 _mdnsresponder(65) mDNSRespo udp *:5353 5353",
		"312/1 _mdnsresponder(65) mDNSRespo udp *:5353 5353",
		"48300/48213 dj(501) node udp *:41234 41234",
	})
}

func TestParseLsofLinux(t *testing.T) {
	// lsof 4.95 on Linux leaves out the 'f' lines when they weren't asked
	// for; a process without a login name falls back to its uid.
	checkRows(t, parseLsofFixture(t, "lsof_tcp_linux.txt", "tcp"), []string{
		"812/1 root(0) sshd tcp *:22 22",
		"812/1 root(0) sshd tcp *:22 22",
		"48213/48190 dj(1000) node tcp 127.0.0.1:3000 3000",
		"9001/1 1001(1001) java tcp6 [::ffff:127.0.0.1]:8080 8080",
	})
}

func TestParseLsofCommandDefaultsToName(t *testing.T) {
	ls, err := parseLsof(strings.NewReader("p7\ncnginx\nn*:80\n"), "tcp")
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != 1 || ls[0].Command != "nginx" || ls[0].UID != -1 || ls[0].User != "" {
		t.Errorf("got %+v", ls)
	}
}

func TestParseLsofErrors(t *testing.T) {
	for name, in := range map[string]string{
		"bad pid":         "pabc\nn*:80\n",
		"file before pid": "n*:80\n",
	} {
		if _, err := parseLsof(strings.NewReader(in), "tcp"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestLsofPort(t *testing.T) {
	for name, want := range map[string]int{
		"*:3000":         3000,
		"127.0.0.1:22":   22,
		"[::1]:5432":     5432,
		"[fe80::1]:8080": 8080,
		"*:*":            0,
		"*:0":            0,
		"a:1->b:2":       0,
		"no-port":        0,
		"*:70000":        0,
	} {
		got, ok := lsofPort(name)
		if ok != (want != 0) || got != want {
			t.Errorf("lsofPort(%q) = %d, %v; want %d", name, got, ok, want)
		}
	}
}

func TestParsePs(t *testing.T) {
	out := "  812 sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups\n" +
		"48213 node /home/dj/app/node_modules/.bin/vite --port 3000\n" +
		"garbage\n\n"
	got := parsePs([]byte(out))
	if len(got) != 2 {
		t.Fatalf("got %d entries: %v", len(got), got)
	}
	if got[812] != "sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups" {
		t.Errorf("812 = %q", got[812])
	}
	if got[48213] != "node /home/dj/app/node_modules/.bin/vite --port 3000" {
		t.Errorf("48213 = %q", got[48213])
	}
}
