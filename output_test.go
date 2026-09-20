package main

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/dan2756/portkill/internal/sockets"
)

func TestSortListeners(t *testing.T) {
	in := []sockets.Listener{
		{Port: 8080, Proto: "tcp", Addr: "0.0.0.0:8080", PID: 30},
		{Port: 22, Proto: "tcp6", Addr: "[::]:22", PID: 10},
		{Port: 3000, Proto: "tcp", Addr: "127.0.0.1:3000", PID: 20},
		{Port: 22, Proto: "tcp", Addr: "0.0.0.0:22", PID: 10},
		{Port: 3000, Proto: "tcp", Addr: "127.0.0.1:3000", PID: 20}, // SO_REUSEPORT twin
		{Port: 3000, Proto: "tcp", Addr: "127.0.0.1:3000", PID: 19},
	}
	got := sortListeners(in)
	want := []string{
		"22 tcp 0.0.0.0:22 10",
		"22 tcp6 [::]:22 10",
		"3000 tcp 127.0.0.1:3000 19",
		"3000 tcp 127.0.0.1:3000 20",
		"8080 tcp 0.0.0.0:8080 30",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d listeners, want %d: %+v", len(got), len(want), got)
	}
	for i, l := range got {
		s := strings.Join([]string{strconv.Itoa(l.Port), l.Proto, l.Addr, strconv.Itoa(l.PID)}, " ")
		if s != want[i] {
			t.Errorf("row %d = %q, want %q", i, s, want[i])
		}
	}
}

func TestRenderTableTruncatesLastColumn(t *testing.T) {
	var b bytes.Buffer
	rows := [][]string{
		{"48213", "dj", "node /home/dj/src/shop/node_modules/.bin/vite --port 3000 --host 127.0.0.1"},
		{"7", "root", "sshd"},
	}
	renderTable(&b, []string{"PID", "USER", "COMMAND"}, rows, 40)
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	want := []string{
		"PID    USER  COMMAND",
		"48213  dj    node /home/dj/src/shop/nod…",
		"7      root  sshd",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	for _, l := range lines {
		if n := len([]rune(l)); n > 40 {
			t.Errorf("line %q is %d runes wide", l, n)
		}
	}

	b.Reset()
	renderTable(&b, []string{"PID", "USER", "COMMAND"}, rows, 0)
	if !strings.Contains(b.String(), "--host 127.0.0.1") {
		t.Errorf("width 0 should not truncate:\n%s", b.String())
	}
}

func TestRenderTableWideCharacters(t *testing.T) {
	var b bytes.Buffer
	rows := [][]string{
		{"48213", "dj", "node /home/dj/プロジェクト/日本語のサーバー/node_modules/.bin/vite --port 3000"},
		{"7", "ユーザー", "sshd"},
	}
	renderTable(&b, []string{"PID", "USER", "COMMAND"}, rows, 40)
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	want := []string{
		"PID    USER      COMMAND",
		"48213  dj        node /home/dj/プロジェ…",
		"7      ユーザー  sshd",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	for _, l := range lines {
		if n := displayWidth(l); n > 40 {
			t.Errorf("line %q is %d columns wide", l, n)
		}
	}
}

func TestDisplayWidth(t *testing.T) {
	for s, want := range map[string]int{
		"node":                     4,
		"日本語":                      6,
		"\uff46\uff55\uff4c\uff4c": 8, // fullwidth "full"
		"cafe\u0301":               4, // e + combining acute accent
		"한국어 server":               13,
		"\U0001f680 launch":        9,
	} {
		if got := displayWidth(s); got != want {
			t.Errorf("displayWidth(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestPrintJSON(t *testing.T) {
	var b bytes.Buffer
	err := printJSON(&b, []sockets.Listener{
		{Port: 3000, Proto: "tcp", Addr: "127.0.0.1:3000", PID: 48213, PPID: 48190, UID: 1000, User: "dj", Name: "node", Command: "node server.js"},
		{Port: 22, Proto: "tcp", Addr: "0.0.0.0:22", UID: 0, User: "root"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, b.String())
	}
	if len(got) != 2 {
		t.Fatalf("got %d objects", len(got))
	}
	if got[0]["pid"] != float64(48213) || got[0]["command"] != "node server.js" || got[0]["address"] != "127.0.0.1:3000" {
		t.Errorf("first object = %v", got[0])
	}
	if got[1]["pid"] != nil || got[1]["uid"] != float64(0) {
		t.Errorf("unknown pid should be null and uid 0 kept: %v", got[1])
	}

	b.Reset()
	if err := printJSON(&b, nil); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(b.String()) != "[]" {
		t.Errorf("empty list = %q, want []", b.String())
	}
}
