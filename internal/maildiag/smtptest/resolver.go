package smtptest

import (
	"context"
	"net"
	"strings"
)

// Resolver is an in-memory maildiag.Resolver. Unknown names answer
// "not found"; names in Fail answer a temporary DNS error.
type Resolver struct {
	Hosts map[string][]string
	TXT   map[string][]string
	MX    map[string][]*net.MX
	Fail  map[string]bool
}

func (r *Resolver) err(name string) error {
	if r.Fail[name] {
		return &net.DNSError{Err: "server misbehaving", Name: name, IsTemporary: true}
	}
	return &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

func key(name string) string { return strings.TrimSuffix(strings.ToLower(name), ".") }

func (r *Resolver) LookupHost(_ context.Context, host string) ([]string, error) {
	if v, ok := r.Hosts[key(host)]; ok && !r.Fail[key(host)] {
		return v, nil
	}
	return nil, r.err(key(host))
}

func (r *Resolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	if v, ok := r.TXT[key(name)]; ok && !r.Fail[key(name)] {
		return v, nil
	}
	return nil, r.err(key(name))
}

func (r *Resolver) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	if v, ok := r.MX[key(name)]; ok && !r.Fail[key(name)] {
		return v, nil
	}
	return nil, r.err(key(name))
}
