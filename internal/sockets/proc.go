package sockets

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Proc finds listening sockets by reading a Linux procfs: the socket tables
// in /proc/net, then every /proc/<pid>/fd to see which process owns which
// socket inode.
type Proc struct {
	Root  string           // procfs mount point; "/proc" when empty
	Order binary.ByteOrder // byte order of /proc/net addresses; the host's when nil
}

// Name implements Backend.
func (p *Proc) Name() string { return "/proc" }

type procTable struct {
	file     string
	proto    string
	optional bool // tcp6 is missing when IPv6 is disabled
}

var procTables = []procTable{
	{"net/tcp", "tcp", false},
	{"net/tcp6", "tcp6", true},
}

type tableSocket struct {
	procSocket
	proto string
}

// Listeners implements Backend.
func (p *Proc) Listeners() ([]Listener, error) {
	var socks []tableSocket
	want := make(map[uint64]bool)
	for _, t := range procTables {
		ss, err := p.readTable(t.file)
		if err != nil {
			if t.optional && errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, s := range ss {
			socks = append(socks, tableSocket{s, t.proto})
			if s.inode != 0 {
				want[s.inode] = true
			}
		}
	}
	if len(socks) == 0 {
		return nil, nil
	}

	owners := p.socketOwners(want)
	users := userCache{}
	procs := map[int]procInfo{}
	var out []Listener
	for _, s := range socks {
		base := Listener{
			Proto: s.proto,
			Addr:  s.addr.String(),
			Port:  int(s.addr.Port()),
			UID:   s.uid,
		}
		pids := owners[s.inode]
		if len(pids) == 0 {
			// The socket exists but its owner's fds aren't readable
			// (another user's process and we aren't root).
			base.User = users.name(s.uid)
			out = append(out, base)
			continue
		}
		for _, pid := range pids {
			info, ok := procs[pid]
			if !ok {
				info = p.process(pid)
				procs[pid] = info
			}
			l := base
			l.PID = pid
			l.PPID = info.ppid
			l.Name = info.name
			l.Command = info.cmdline
			if l.Command == "" {
				l.Command = info.name
			}
			if info.uid >= 0 {
				l.UID = info.uid
			}
			l.User = users.name(l.UID)
			out = append(out, l)
		}
	}
	return out, nil
}

func (p *Proc) root() string {
	if p.Root == "" {
		return "/proc"
	}
	return p.Root
}

func (p *Proc) order() binary.ByteOrder {
	if p.Order == nil {
		return binary.NativeEndian
	}
	return p.Order
}

func (p *Proc) readTable(name string) ([]procSocket, error) {
	f, err := os.Open(filepath.Join(p.root(), name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	ss, err := parseProcNet(f, p.order())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", f.Name(), err)
	}
	return ss, nil
}

// socketOwners walks /proc/<pid>/fd and returns, for every wanted socket
// inode, the pids that have it open. Processes whose fds we can't read are
// skipped.
func (p *Proc) socketOwners(want map[uint64]bool) map[uint64][]int {
	owners := make(map[uint64][]int)
	entries, err := os.ReadDir(p.root())
	if err != nil {
		return owners
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		fdDir := filepath.Join(p.root(), e.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue // exited, or not ours to look at
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			inode, ok := socketInode(link)
			if !ok || !want[inode] {
				continue
			}
			pids := owners[inode]
			if len(pids) > 0 && pids[len(pids)-1] == pid {
				continue // same socket on another fd of this process
			}
			owners[inode] = append(pids, pid)
		}
	}
	return owners
}

// socketInode extracts the inode from an fd link such as "socket:[41234]".
func socketInode(link string) (uint64, bool) {
	s, ok := strings.CutPrefix(link, "socket:[")
	if !ok {
		return 0, false
	}
	s, ok = strings.CutSuffix(s, "]")
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil
}

type procInfo struct {
	name    string
	cmdline string
	ppid    int
	uid     int
}

func (p *Proc) process(pid int) procInfo {
	dir := filepath.Join(p.root(), strconv.Itoa(pid))
	info := procInfo{uid: -1}
	if f, err := os.Open(filepath.Join(dir, "status")); err == nil {
		info = parseStatus(f)
		f.Close()
	}
	if b, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
		info.cmdline = parseCmdline(b)
	}
	return info
}

// parseStatus pulls the process name, parent pid and effective uid out of
// /proc/<pid>/status.
func parseStatus(r io.Reader) procInfo {
	info := procInfo{uid: -1}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		key, val, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch key {
		case "Name":
			info.name = val
		case "PPid":
			info.ppid, _ = strconv.Atoi(val)
		case "Uid":
			// real, effective, saved, filesystem
			if f := strings.Fields(val); len(f) >= 2 {
				if uid, err := strconv.Atoi(f[1]); err == nil {
					info.uid = uid
				}
			}
		}
	}
	return info
}

// parseCmdline turns the NUL-separated /proc/<pid>/cmdline into a single
// space-separated string. Kernel threads have an empty cmdline.
func parseCmdline(b []byte) string {
	b = bytes.TrimRight(b, "\x00")
	return strings.TrimSpace(string(bytes.ReplaceAll(b, []byte{0}, []byte{' '})))
}
