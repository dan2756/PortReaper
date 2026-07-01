package main

import (
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"

	"github.com/dan2756/portkill/internal/sockets"
)

func printTable(w io.Writer, ls []sockets.Listener) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PID\tUSER\tPROTO\tADDRESS\tCOMMAND")
	for _, l := range ls {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", pidString(l.PID), orDash(l.User), l.Proto, l.Addr, command(l))
	}
	tw.Flush()
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
		return "(not visible; try sudo)"
	}
	return "-"
}
