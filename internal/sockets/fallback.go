package sockets

// Fallback asks Primary first and Secondary only when Primary fails or sees
// no listening sockets at all, which happens in some containers and
// sandboxes that hide or empty /proc/net.
type Fallback struct {
	Primary   Backend
	Secondary Backend // optional

	last Backend // whichever answered the last call
}

// Name implements Backend: the name of the backend that answered last.
func (f *Fallback) Name() string {
	if f.last != nil {
		return f.last.Name()
	}
	return f.Primary.Name()
}

// Listeners implements Backend.
func (f *Fallback) Listeners(opts Options) ([]Listener, error) {
	f.last = f.Primary
	ls, err := f.Primary.Listeners(opts)
	if (err == nil && len(ls) > 0) || f.Secondary == nil {
		return ls, err
	}
	ls2, err2 := f.Secondary.Listeners(opts)
	if err2 != nil || (err == nil && len(ls2) == 0) {
		// Keep the primary's answer: its error is the one worth showing,
		// "nothing listening" shouldn't turn into a failure, and when
		// neither sees a socket the primary's empty table stands, so
		// HidesOtherUsers doesn't send anyone to sudo for nothing.
		return ls, err
	}
	f.last = f.Secondary
	return ls2, nil
}

// HidesOtherUsers reports whether the backend that answered last can miss
// other users' sockets.
func (f *Fallback) HidesOtherUsers() bool {
	h, ok := f.last.(interface{ HidesOtherUsers() bool })
	return ok && h.HidesOtherUsers()
}
