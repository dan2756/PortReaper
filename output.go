package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

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
// is positive the last column is cut so that lines fit in it.
func renderTable(w io.Writer, header []string, rows [][]string, width int) {
	widths := make([]int, len(header))
	for _, r := range append([][]string{header}, rows...) {
		for i, cell := range r {
			widths[i] = max(widths[i], utf8.RuneCountInString(cell))
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
			b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell)+2))
		}
		fmt.Fprintln(w, b.String())
	}
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
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
