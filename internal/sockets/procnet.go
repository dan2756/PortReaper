package sockets

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/netip"
	"strconv"
	"strings"
)

// Socket states from include/net/tcp_states.h. UDP sockets reuse them:
// an unconnected UDP socket is in TCP_CLOSE.
const (
	tcpListen = 0x0A
	tcpClose  = 0x07
)

// procSocket is one row of /proc/net/{tcp,tcp6,udp,udp6}.
type procSocket struct {
	addr  netip.AddrPort
	uid   int
	inode uint64
}

// parseProcNet reads a /proc/net socket table and returns the listening
// sockets: TCP sockets in LISTEN state when udp is false, unconnected UDP
// sockets bound to a port when it is true. order is the byte order the kernel
// used when it printed the addresses, which is the host's.
func parseProcNet(r io.Reader, udp bool, order binary.ByteOrder) ([]procSocket, error) {
	var out []procSocket
	sc := bufio.NewScanner(r)
	line := 0
	for sc.Scan() {
		line++
		if line == 1 {
			continue // header
		}
		f := strings.Fields(sc.Text())
		if len(f) == 0 {
			continue
		}
		// sl local_address rem_address st tx_queue:rx_queue tr:tm->when retrnsmt uid timeout inode ...
		if len(f) < 10 {
			return nil, fmt.Errorf("line %d: expected at least 10 fields, got %d", line, len(f))
		}
		state, err := strconv.ParseUint(f[3], 16, 8)
		if err != nil {
			return nil, fmt.Errorf("line %d: bad state %q", line, f[3])
		}
		if (!udp && state != tcpListen) || (udp && state != tcpClose) {
			continue
		}
		addr, err := parseHexAddrPort(f[1], order)
		if err != nil {
			return nil, fmt.Errorf("line %d: %v", line, err)
		}
		if udp {
			remote, err := parseHexAddrPort(f[2], order)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", line, err)
			}
			if remote.Port() != 0 || addr.Port() == 0 {
				continue // connected, or not bound to a port yet
			}
		}
		uid, err := strconv.Atoi(f[7])
		if err != nil {
			return nil, fmt.Errorf("line %d: bad uid %q", line, f[7])
		}
		inode, err := strconv.ParseUint(f[9], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("line %d: bad inode %q", line, f[9])
		}
		out = append(out, procSocket{addr: addr, uid: uid, inode: inode})
	}
	return out, sc.Err()
}

// parseHexAddrPort decodes an address such as "0100007F:0BB8". The port is
// printed in network order; the address is printed as 32-bit words in host
// order.
func parseHexAddrPort(s string, order binary.ByteOrder) (netip.AddrPort, error) {
	host, port, ok := strings.Cut(s, ":")
	if !ok {
		return netip.AddrPort{}, fmt.Errorf("bad address %q", s)
	}
	p, err := strconv.ParseUint(port, 16, 16)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("bad port in %q", s)
	}
	b, err := hex.DecodeString(host)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("bad address %q", s)
	}
	var addr netip.Addr
	switch len(b) {
	case 4:
		var a [4]byte
		order.PutUint32(a[:], binary.BigEndian.Uint32(b))
		addr = netip.AddrFrom4(a)
	case 16:
		addr = netip.AddrFrom16([16]byte(b))
	default:
		return netip.AddrPort{}, fmt.Errorf("bad address length in %q", s)
	}
	return netip.AddrPortFrom(addr, uint16(p)), nil
}
