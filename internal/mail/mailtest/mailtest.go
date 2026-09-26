// Package mailtest provides a recording mail.Mailer for tests.
package mailtest

import (
	"context"
	"sync"

	"github.com/qwerin/nanofaktura/internal/mail"
)

// Recorder is a mail.Mailer that stores sent messages instead of sending them.
// Set Err to make Send fail (the message is then not recorded).
type Recorder struct {
	mu   sync.Mutex
	msgs []mail.Message
	Err  error
}

func New() *Recorder { return &Recorder{} }

func (r *Recorder) Send(_ context.Context, m mail.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.msgs = append(r.msgs, m)
	return nil
}

// Messages returns a copy of all recorded messages, oldest first.
func (r *Recorder) Messages() []mail.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]mail.Message(nil), r.msgs...)
}

// Last returns the most recent message; ok is false when nothing was sent.
func (r *Recorder) Last() (m mail.Message, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.msgs) == 0 {
		return mail.Message{}, false
	}
	return r.msgs[len(r.msgs)-1], true
}

// Reset forgets all recorded messages.
func (r *Recorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = nil
}
