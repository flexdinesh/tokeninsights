package accounts

import (
	"net"
	"sync"
	"time"
)

const (
	BatchRequestsPerSecond = 10
	BatchBurst             = 20
	LoginRequestsPerMinute = 5
	maxLoginAddresses      = 1024
	loginAddressRetention  = 10 * time.Minute
)

type AdmissionError struct {
	Code       string
	RetryAfter int
}

func (e *AdmissionError) Error() string { return e.Code }

type bucket struct {
	tokens  float64
	updated time.Time
	active  bool
}
type admission struct {
	sync.Mutex
	users map[string]*bucket
}

func newAdmission() *admission { return &admission{users: make(map[string]*bucket)} }

// Admit bounds expensive hosted acceptance before decoding. The engine supplies
// the additional global limit; this gate prevents one user monopolizing it.
func (s *Service) Admit(p Principal) (func(), error) {
	a := s.admission
	a.Lock()
	defer a.Unlock()
	now := time.Now()
	b := a.users[p.UserID]
	if b == nil {
		b = &bucket{tokens: BatchBurst, updated: now}
		a.users[p.UserID] = b
	}
	b.tokens = min(BatchBurst, b.tokens+now.Sub(b.updated).Seconds()*BatchRequestsPerSecond)
	b.updated = now
	if b.active {
		return nil, &AdmissionError{Code: "user_busy", RetryAfter: 1}
	}
	if b.tokens < 1 {
		return nil, &AdmissionError{Code: "rate_limited", RetryAfter: 1}
	}
	b.tokens--
	b.active = true
	var once sync.Once
	return func() { once.Do(func() { a.Lock(); b.active = false; a.Unlock() }) }, nil
}

type loginAdmission struct {
	sync.Mutex
	clients map[string]*bucket
}

func newLoginAdmission() *loginAdmission { return &loginAdmission{clients: make(map[string]*bucket)} }

// AdmitLogin consumes the client address resolved by the HTTP boundary.
// Account policy never reads proxy headers or deployment configuration.
func (s *Service) AdmitLogin(remoteAddr string) error {
	address, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		address = remoteAddr
	}
	a := s.login
	a.Lock()
	defer a.Unlock()
	now := time.Now()
	for key, b := range a.clients {
		if now.Sub(b.updated) > loginAddressRetention {
			delete(a.clients, key)
		}
	}
	b := a.clients[address]
	if b == nil {
		if len(a.clients) >= maxLoginAddresses {
			return &AdmissionError{Code: "rate_limited", RetryAfter: 60}
		}
		b = &bucket{tokens: LoginRequestsPerMinute, updated: now}
		a.clients[address] = b
	}
	b.tokens = min(LoginRequestsPerMinute, b.tokens+now.Sub(b.updated).Minutes()*LoginRequestsPerMinute)
	b.updated = now
	if b.tokens < 1 {
		return &AdmissionError{Code: "rate_limited", RetryAfter: 60}
	}
	b.tokens--
	return nil
}
