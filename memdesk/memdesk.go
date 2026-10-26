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
	Status     supportcore.Status
	Sent       []Email               // what the requester was sent
	Transcript []supportcore.Message // what responders see
}

// Desk is an in-memory [supportcore.Desk] that implements every optional
// extension. It is safe for concurrent use.
type Desk struct {
	name string
	now  func() time.Time

	mu      sync.Mutex
	seq     int
	tickets map[string]*Ticket
	fail    []error
	portal  string
}

// SetPortal makes [Desk.TicketURL] return base followed by the ticket ID.
// With no portal set, it returns "".
func (d *Desk) SetPortal(base string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.portal = base
}

// SetStatus sets a ticket's status, as a responder would at the desk. It
// reports false if the ticket does not exist.
func (d *Desk) SetStatus(id string, s supportcore.Status) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, ok := d.tickets[id]
	if ok {
		t.Status = s
	}
	return ok
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
	t := &Ticket{Case: c, Status: supportcore.StatusOpen}
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
	t, err := d.begin(ctx, del)
	if err != nil {
		return supportcore.Transcript{}, err
	}
	defer d.mu.Unlock()
	return supportcore.Transcript{Delivery: del, Messages: slices.Clone(t.Transcript)}, nil
}

// TicketURL implements [supportcore.TicketLinker].
func (d *Desk) TicketURL(ctx context.Context, del supportcore.Delivery) (string, error) {
	if _, err := d.begin(ctx, del); err != nil {
		return "", err
	}
	defer d.mu.Unlock()
	if d.portal == "" {
		return "", nil
	}
	return d.portal + del.TicketID, nil
}

// TicketStatus implements [supportcore.TicketStatuser].
func (d *Desk) TicketStatus(ctx context.Context, del supportcore.Delivery) (supportcore.Status, error) {
	t, err := d.begin(ctx, del)
	if err != nil {
		return "", err
	}
	defer d.mu.Unlock()
	return t.Status, nil
}

// Reply implements [supportcore.TicketReplier]. Like a vendor, the desk
// replies to the address it holds for the ticket and ignores c. The reply is
// emailed to the requester, joins the transcript, and leaves the ticket
// pending.
func (d *Desk) Reply(ctx context.Context, del supportcore.Delivery, _ supportcore.Case, body string) error {
	t, err := d.begin(ctx, del)
	if err != nil {
		return err
	}
	defer d.mu.Unlock()
	if t.Case.Requester.ReplyTo == "" {
		return fmt.Errorf("memdesk: ticket %s has no reply address: %w", del.TicketID, supportcore.ErrRejected)
	}
	t.Sent = append(t.Sent, Email{To: t.Case.Requester.ReplyTo, Subject: "Re: " + t.Case.Reference, Body: body})
	t.Transcript = append(t.Transcript, supportcore.Message{Author: supportcore.AuthorResponder, Body: body, SentAt: d.now()})
	t.Status = supportcore.StatusPending
	return nil
}

// begin checks ctx, takes d.mu, consumes an injected error and finds the
// ticket. On success the caller holds d.mu and must unlock it.
func (d *Desk) begin(ctx context.Context, del supportcore.Delivery) (*Ticket, error) {
	if err := ctx.Err(); err != nil {
		return nil, supportcore.Retry(err, 0)
	}
	d.mu.Lock()
	if err := d.popFail(); err != nil {
		d.mu.Unlock()
		return nil, err
	}
	t, ok := d.tickets[del.TicketID]
	if !ok {
		d.mu.Unlock()
		return nil, fmt.Errorf("memdesk: ticket %s: %w", del.TicketID, supportcore.ErrRejected)
	}
	return t, nil
}

// Ticket returns a copy of the ticket with the given ID.
func (d *Desk) Ticket(id string) (Ticket, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, ok := d.tickets[id]
	if !ok {
		return Ticket{}, false
	}
	return Ticket{Case: t.Case, Status: t.Status, Sent: slices.Clone(t.Sent), Transcript: slices.Clone(t.Transcript)}, true
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
