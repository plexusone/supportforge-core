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
