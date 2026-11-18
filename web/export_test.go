package web

// Holding is whether a request has entered the gentle and not yet left.
// The tests use it to see that a body holds the gate until it is closed.
func Holding(gentle *Gentle) bool { return len(gentle.turn) > 0 }

// SwapDefaults replaces the bytes of the default numbers file, so a test
// sees what happens when it will not read. It gives a way to put it back.
func SwapDefaults(raw []byte) (restore func()) {
	old := numbersRaw
	numbersRaw = raw
	return func() { numbersRaw = old }
}

// NewStatus is the refusal a client gives for an answer of a status and a
// Retry-After, made without a server, so a test needs none.
func NewStatus(method, address, status string, code int,
	retryAfter string) *Status {
	return &Status{method: method, url: address, status: status,
		code: code, retryAfter: retryAfter}
}
