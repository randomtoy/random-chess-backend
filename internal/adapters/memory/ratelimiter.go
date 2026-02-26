package memory

// AlwaysAllow is a stub RateLimiter that permits every request.
type AlwaysAllow struct{}

func (AlwaysAllow) Allow(_, _ string) bool { return true }
