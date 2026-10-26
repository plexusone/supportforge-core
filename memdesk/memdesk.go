// Package memdesk is an in-memory desk for tests. It records what a real
// desk would send and show, and returns injected errors on demand.
package memdesk

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	supportcore "github.com/plexusone/supportforge-core"
)

// Email is a message the desk sent to a requester.
type Email struct {
	To      string
	Subject string
	Body    string
}

// Ticket is what the desk holds for one delivered case.
type Ticket struct {
	Case       supportcore.Case
	Sent       []Email               // what the requester was sent
	Transcript []supportcore.Message // what responders see
}

// Desk is an in-memory [supportcore.Desk] that also implements
// [supportcore.TicketEraser] and [supportcore.TicketExporter]. It is safe
// for concurrent use.
type Desk struct {
	name string
	now  func() time.Time

	mu      sync.Mutex
	seq     int
	tickets map[string]*Ticket
	fail    []error
}

// New returns an empty desk with the given name, "memory" if empty.
func New(name string) *Desk {
	if name == "" {
		name = "memory"
	}
	return &Desk{name: name, now: time.Now, tickets: map[string]*Ticket{}}
}

// Name implements [supportcore.Desk].
func (d *Desk) Name() string { return d.name }

// FailNext makes the next call to Deliver, EraseTicket or ExportTicket
// return err instead of acting. Several calls queue several errors.
func (d *Desk) FailNext(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.fail = append(d.fail, err)
}

// popFail returns the next injected error. The caller holds d.mu.
func (d *Desk) popFail() error {
	if len(d.fail) == 0 {
		return nil
	}
	err := d.fail[0]
	d.fail = d.fail[1:]
	return err
}

// Deliver implements [supportcore.Desk]. The case's text and the context go
// to the transcript, and the acknowledgment is the only email sent.
func (d *Desk) Deliver(ctx context.Context, c supportcore.Case, ack supportcore.Acknowledgment, private supportcore.Context) (supportcore.Delivery, error) {
	if err := ctx.Err(); err != nil {
		return supportcore.Delivery{}, supportcore.Retry(err, 0)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.popFail(); err != nil {
		return supportcore.Delivery{}, err
	}
	now := d.now()
	t := &Ticket{Case: c}
	t.Transcript = append(t.Transcript, supportcore.Message{
		Author: supportcore.AuthorRequester,
		Body:   c.Title + "\n\n" + c.Body,
		SentAt: c.CreatedAt,
	})
	if len(private) > 0 {
		t.Transcript = append(t.Transcript, supportcore.Message{
			Author: supportcore.AuthorNote,
			Body:   renderContext(private),
			SentAt: now,
		})
	}
	if c.Requester.ReplyTo != "" {
		t.Sent = append(t.Sent, Email{To: c.Requester.ReplyTo, Subject: ack.Subject, Body: ack.Body})
		t.Transcript = append(t.Transcript, supportcore.Message{
			Author: supportcore.AuthorResponder,
			Body:   ack.Body,
			SentAt: now,
		})
	}
	d.seq++
	id := "mem-" + strconv.Itoa(d.seq)
	d.tickets[id] = t
	return supportcore.Delivery{Desk: d.name, TicketID: id}, nil
}

// EraseTicket implements [supportcore.TicketEraser].
func (d *Desk) EraseTicket(ctx context.Context, del supportcore.Delivery) error {
	if err := ctx.Err(); err != nil {
		return supportcore.Retry(err, 0)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.popFail(); err != nil {
		return err
	}
	delete(d.tickets, del.TicketID)
	return nil
}

// ExportTicket implements [supportcore.TicketExporter].
func (d *Desk) ExportTicket(ctx context.Context, del supportcore.Delivery) (supportcore.Transcript, error) {
	if err := ctx.Err(); err != nil {
		return supportcore.Transcript{}, supportcore.Retry(err, 0)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.popFail(); err != nil {
		return supportcore.Transcript{}, err
	}
	t, ok := d.tickets[del.TicketID]
	if !ok {
		return supportcore.Transcript{}, fmt.Errorf("memdesk: ticket %s: %w", del.TicketID, supportcore.ErrRejected)
	}
	return supportcore.Transcript{Delivery: del, Messages: slices.Clone(t.Transcript)}, nil
}

// Ticket returns a copy of the ticket with the given ID.
func (d *Desk) Ticket(id string) (Ticket, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, ok := d.tickets[id]
	if !ok {
		return Ticket{}, false
	}
	return Ticket{Case: t.Case, Sent: slices.Clone(t.Sent), Transcript: slices.Clone(t.Transcript)}, true
}

// Len returns the number of tickets the desk holds.
func (d *Desk) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.tickets)
}

func renderContext(c supportcore.Context) string {
	var b strings.Builder
	for _, k := range slices.Sorted(maps.Keys(c)) {
		b.WriteString(k + ": " + c[k] + "\n")
	}
	return b.String()
}
