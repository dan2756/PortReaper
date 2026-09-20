package sockets

import (
	"errors"
	"testing"
)

type stubBackend struct {
	name  string
	ls    []Listener
	err   error
	calls int
}

func (s *stubBackend) Name() string { return s.name }

func (s *stubBackend) Listeners(Options) ([]Listener, error) {
	s.calls++
	return s.ls, s.err
}

func TestFallback(t *testing.T) {
	some := []Listener{{Proto: "tcp", Addr: "127.0.0.1:3000", Port: 3000, PID: 7}}
	broken := errors.New("open /proc/net/tcp: permission denied")

	for _, tc := range []struct {
		name          string
		primary       *stubBackend
		secondary     *stubBackend
		wantN         int
		wantErr       bool
		wantName      string
		wantSecondary int // calls
	}{
		{"primary has sockets", &stubBackend{name: "/proc", ls: some}, &stubBackend{name: "lsof", ls: some}, 1, false, "/proc", 0},
		{"primary empty", &stubBackend{name: "/proc"}, &stubBackend{name: "lsof", ls: some}, 1, false, "lsof", 1},
		{"primary fails", &stubBackend{name: "/proc", err: broken}, &stubBackend{name: "lsof", ls: some}, 1, false, "lsof", 1},
		{"both empty", &stubBackend{name: "/proc"}, &stubBackend{name: "lsof"}, 0, false, "/proc", 1},
		{"primary fails, lsof empty", &stubBackend{name: "/proc", err: broken}, &stubBackend{name: "lsof"}, 0, false, "lsof", 1},
		{"both fail", &stubBackend{name: "/proc", err: broken}, &stubBackend{name: "lsof", err: errors.New("lsof: exit status 2")}, 0, true, "/proc", 1},
		{"lsof fails, nothing listening", &stubBackend{name: "/proc"}, &stubBackend{name: "lsof", err: errors.New("lsof: exit status 2")}, 0, false, "/proc", 1},
		{"no lsof", &stubBackend{name: "/proc", err: broken}, nil, 0, true, "/proc", 0},
	} {
		f := &Fallback{Primary: tc.primary}
		if tc.secondary != nil {
			f.Secondary = tc.secondary
		}
		ls, err := f.Listeners(Options{})
		if len(ls) != tc.wantN || (err != nil) != tc.wantErr {
			t.Errorf("%s: got %d listeners, err %v; want %d, error %v", tc.name, len(ls), err, tc.wantN, tc.wantErr)
		}
		if tc.wantErr && !errors.Is(err, broken) {
			t.Errorf("%s: error %v should be the primary's", tc.name, err)
		}
		if f.Name() != tc.wantName {
			t.Errorf("%s: Name() = %q, want %q", tc.name, f.Name(), tc.wantName)
		}
		if tc.secondary != nil && tc.secondary.calls != tc.wantSecondary {
			t.Errorf("%s: secondary called %d times, want %d", tc.name, tc.secondary.calls, tc.wantSecondary)
		}
	}
}

// hidingStub is a stub that, like lsof without root, can miss other users'
// sockets.
type hidingStub struct{ stubBackend }

func (*hidingStub) HidesOtherUsers() bool { return true }

func TestFallbackHidesOtherUsers(t *testing.T) {
	f := &Fallback{Primary: &stubBackend{name: "/proc"}, Secondary: &Lsof{Path: "/nonexistent/lsof"}}
	f.Listeners(Options{}) // lsof fails, /proc answered
	if f.HidesOtherUsers() {
		t.Error("/proc answered; it sees every socket")
	}
	f.last = f.Secondary
	if !f.HidesOtherUsers() {
		t.Error("lsof answered; it can miss other users' sockets")
	}

	// An empty /proc table means nothing is listening, whatever lsof says.
	f = &Fallback{Primary: &stubBackend{name: "/proc"}, Secondary: &hidingStub{stubBackend{name: "lsof"}}}
	f.Listeners(Options{})
	if f.HidesOtherUsers() {
		t.Error("neither saw a socket; /proc's empty table is the answer")
	}
}
