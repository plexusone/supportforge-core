package supportcore

import "context"

// Desk delivers a case to a help desk, or to a mailbox, where responders
// answer it.
//
// The host stores a case before calling Deliver and calls it from a worker,
// so Deliver may run long after intake and may be retried. A desk keeps
// these rules, which desktest checks:
//
//   - The requester is sent ack and nothing else: not the case's title or
//     body, and not the private context. Whether ack quotes the requester's
//     text is the host's decision.
//   - When the requester has no reply address, nothing is sent to them; a
//     desk that cannot create a ticket without one returns [ErrRejected].
//   - Responders see the case's title and body and the private context.
//   - The returned [Delivery] names this desk and the desk's own ticket ID.
//   - Every error is classified, by wrapping one of [ErrRetryable],
//     [ErrCredential], [ErrRejected] or [ErrUnsupported].
//
// Deliver may be called again for a case it already delivered, when a
// failure came after the ticket was created. A desk whose vendor accepts an
// idempotency key uses the case's ID as that key.
type Desk interface {
	// Name identifies the desk in configuration, in logs and in
	// [Delivery.Desk]. It is stable and lowercase, such as "mail".
	Name() string

	// Deliver creates a ticket for c, sends ack to the requester, and shows
	// private to responders only.
	Deliver(ctx context.Context, c Case, ack Acknowledgment, private Context) (Delivery, error)
}

// TicketEraser is implemented by a desk that can delete a ticket, so that a
// requester's erasure reaches the desk. Erasing a ticket that no longer
// exists succeeds.
type TicketEraser interface {
	EraseTicket(ctx context.Context, d Delivery) error
}

// TicketExporter is implemented by a desk that can return a ticket's
// conversation, so that a requester's data export includes it.
type TicketExporter interface {
	ExportTicket(ctx context.Context, d Delivery) (Transcript, error)
}

// TicketLinker is implemented by a desk that has a page where the requester
// can sign in and follow their ticket, such as a help center. The URL
// carries no personal data. A desk returns "" for a ticket with no such
// page.
type TicketLinker interface {
	TicketURL(ctx context.Context, d Delivery) (string, error)
}

// TicketStatuser is implemented by a desk that can report a ticket's
// status, so the requester's list is current and the host knows when a
// conversation has ended.
type TicketStatuser interface {
	TicketStatus(ctx context.Context, d Delivery) (Status, error)
}

// TicketReplier is implemented by a desk that can send a reply into a
// ticket's conversation. The body goes to the requester, and responders see
// it in the ticket. The host passes the case as it holds it, so a desk with
// no ticket store of its own, such as mail, can address the reply; its reply
// address is empty once the host has cleared it. A desk returns
// [ErrRejected] when the ticket no longer exists or there is no address to
// reply to.
type TicketReplier interface {
	Reply(ctx context.Context, d Delivery, c Case, body string) error
}
