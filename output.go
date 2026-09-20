package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/dan2756/portkill/internal/sockets"
)

// printTable prints the listeners on the ports being killed.
func printTable(w io.Writer, ls []sockets.Listener, width int) {
	rows := make([][]string, len(ls))
	for i, l := range ls {
		rows[i] = []string{pidString(l.PID), orDash(l.User), l.Proto, l.Addr, command(l)}
	}
	renderTable(w, []string{"PID", "USER", "PROTO", "ADDRESS", "COMMAND"}, rows, width)
}

// printList prints every listener, one row per socket and process, sorted by
// port.
func printList(w io.Writer, ls []sockets.Listener, width int) {
	rows := make([][]string, len(ls))
	for i, l := range ls {
		rows[i] = []string{strconv.Itoa(l.Port), l.Proto, l.Addr, pidString(l.PID), orDash(l.User), command(l)}
	}
	renderTable(w, []string{"PORT", "PROTO", "ADDRESS", "PID", "USER", "COMMAND"}, rows, width)
}

// renderTable prints left-aligned columns separated by two spaces. When width
// is positive the last column is cut so that lines fit in that many terminal
// columns.
func renderTable(w io.Writer, header []string, rows [][]string, width int) {
	widths := make([]int, len(header))
	for _, r := range append([][]string{header}, rows...) {
		for i, cell := range r {
			widths[i] = max(widths[i], displayWidth(cell))
		}
	}
	last := len(header) - 1
	room := 0
	if width > 0 {
		room = width
		for _, cw := range widths[:last] {
			room -= cw + 2
		}
		room = max(room, 20) // a cramped terminal still gets a usable command
	}
	var b strings.Builder
	for _, r := range append([][]string{header}, rows...) {
		b.Reset()
		for i, cell := range r {
			if i == last {
				if room > 0 {
					cell = truncate(cell, room)
				}
				b.WriteString(cell)
				break
			}
			b.WriteString(cell)
			b.WriteString(strings.Repeat(" ", widths[i]-displayWidth(cell)+2))
		}
		fmt.Fprintln(w, b.String())
	}
}

// truncate cuts s to at most n terminal columns, ending it with an ellipsis
// when something was cut.
func truncate(s string, n int) string {
	if displayWidth(s) <= n {
		return s
	}
	w := 0
	for i, r := range s {
		if w += runeWidth(r); w > n-1 {
			return s[:i] + "…"
		}
	}
	return s
}

// displayWidth is the number of terminal columns s takes up.
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		n += runeWidth(r)
	}
	return n
}

// runeWidth is the number of terminal columns r takes up: two for East Asian
// wide and fullwidth characters, none for combining marks, one otherwise.
func runeWidth(r rune) int {
	switch {
	case unicode.In(r, unicode.Mn, unicode.Me):
		return 0
	case unicode.Is(wide, r):
		return 2
	}
	return 1
}

// wide holds the blocks that terminals draw two columns wide: CJK, Hangul,
// fullwidth forms and emoji.
var wide = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x1100, Hi: 0x115f, Stride: 1},
		{Lo: 0x2329, Hi: 0x232a, Stride: 1},
		{Lo: 0x2e80, Hi: 0x303e, Stride: 1},
		{Lo: 0x3041, Hi: 0x33ff, Stride: 1},
		{Lo: 0x3400, Hi: 0x4dbf, Stride: 1},
		{Lo: 0x4e00, Hi: 0x9fff, Stride: 1},
		{Lo: 0xa000, Hi: 0xa4cf, Stride: 1},
		{Lo: 0xa960, Hi: 0xa97f, Stride: 1},
		{Lo: 0xac00, Hi: 0xd7a3, Stride: 1},
		{Lo: 0xf900, Hi: 0xfaff, Stride: 1},
		{Lo: 0xfe10, Hi: 0xfe19, Stride: 1},
		{Lo: 0xfe30, Hi: 0xfe6f, Stride: 1},
		{Lo: 0xff00, Hi: 0xff60, Stride: 1},
		{Lo: 0xffe0, Hi: 0xffe6, Stride: 1},
	},
	R32: []unicode.Range32{
		{Lo: 0x1b000, Hi: 0x1b2ff, Stride: 1},
		{Lo: 0x1f300, Hi: 0x1f64f, Stride: 1},
		{Lo: 0x1f680, Hi: 0x1f6ff, Stride: 1},
		{Lo: 0x1f900, Hi: 0x1f9ff, Stride: 1},
		{Lo: 0x20000, Hi: 0x2fffd, Stride: 1},
		{Lo: 0x30000, Hi: 0x3fffd, Stride: 1},
	},
}

type jsonListener struct {
	Port    int    `json:"port"`
	Proto   string `json:"proto"`
	Address string `json:"address"`
	PID     *int   `json:"pid"`
	PPID    int    `json:"ppid,omitempty"`
	User    string `json:"user,omitempty"`
	UID     *int   `json:"uid"`
	Name    string `json:"name,omitempty"`
	Command string `json:"command,omitempty"`
}

// printJSON writes the listeners as a JSON array. Unknown pids and uids are
// null.
func printJSON(w io.Writer, ls []sockets.Listener) error {
	out := make([]jsonListener, 0, len(ls))
	for _, l := range ls {
		j := jsonListener{
			Port:    l.Port,
			Proto:   l.Proto,
			Address: l.Addr,
			PPID:    l.PPID,
			User:    l.User,
			Name:    l.Name,
			Command: l.Command,
		}
		if l.PID > 0 {
			pid := l.PID
			j.PID = &pid
		}
		if l.UID >= 0 {
			uid := l.UID
			j.UID = &uid
		}
		out = append(out, j)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// sortListeners orders listeners by port, protocol, address and pid, and
// drops exact duplicates (the same process holding several sockets on one
// address, e.g. with SO_REUSEPORT).
func sortListeners(ls []sockets.Listener) []sockets.Listener {
	sort.SliceStable(ls, func(i, j int) bool {
		a, b := ls[i], ls[j]
		if a.Port != b.Port {
			return a.Port < b.Port
		}
		if a.Proto != b.Proto {
			return a.Proto < b.Proto
		}
		if a.Addr != b.Addr {
			return a.Addr < b.Addr
		}
		return a.PID < b.PID
	})
	out := ls[:0]
	for _, l := range ls {
		if n := len(out); n > 0 {
			p := out[n-1]
			if p.Port == l.Port && p.Proto == l.Proto && p.Addr == l.Addr && p.PID == l.PID {
				continue
			}
		}
		out = append(out, l)
	}
	return out
}

func pidString(pid int) string {
	if pid <= 0 {
		return "-"
	}
	return strconv.Itoa(pid)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func command(l sockets.Listener) string {
	switch {
	case l.Command != "":
		return l.Command
	case l.Name != "":
		return l.Name
	case l.PID <= 0:
		return "(owner not visible)"
	}
	return "-"
}
