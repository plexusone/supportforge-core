package supportcore

import (
	"log/slog"
	"time"
)

// Kind is what a request is about. It routes the request and decides who may
// see it.
type Kind string

const (
	KindQuestion Kind = "question"
	KindProblem  Kind = "problem"
	KindBilling  Kind = "billing"
	KindFeedback Kind = "feedback"
	KindOther    Kind = "other"
)

// Kinds returns every kind, in the order a form lists them.
func Kinds() []Kind {
	return []Kind{KindQuestion, KindProblem, KindBilling, KindFeedback, KindOther}
}

// Valid reports whether k is one of [Kinds].
func (k Kind) Valid() bool {
	switch k {
	case KindQuestion, KindProblem, KindBilling, KindFeedback, KindOther:
		return true
	}
	return false
}

// Requester is who sent a request.
type Requester struct {
	PrincipalID    string // empty for a visitor
	OrganizationID string // empty for a visitor
	ReplyTo        string // email address; may be empty for feedback with no reply wanted
}

// Visitor reports whether the requester was not signed in.
func (r Requester) Visitor() bool {
	return r.PrincipalID == ""
}

// String describes the requester without personal data, so that formatting
// one with %v cannot leak an address into a log.
func (r Requester) String() string {
	if r.Visitor() {
		return "visitor"
	}
	return "signed-in requester"
}

// LogValue implements [slog.LogValuer] without personal data.
func (r Requester) LogValue() slog.Value {
	return slog.GroupValue(slog.Bool("visitor", r.Visitor()))
}

// Case is a support request or feedback, as accepted by the host.
type Case struct {
	ID          string
	Reference   string // short, shown to the requester
	Product     string
	Kind        Kind
	Title       string
	Body        string
	Page        string // a path only
	ReplyWanted bool
	Shared      bool // visible in full to the requester's organization administrators
	Requester   Requester
	CreatedAt   time.Time
}

// String identifies the case without its text or the requester's address,
// so that formatting one with %v cannot leak them into a log.
func (c Case) String() string {
	return "case " + c.Reference + " (" + string(c.Kind) + ", " + c.Product + ")"
}

// GoString is [Case.String], so %#v does not print the text either.
func (c Case) GoString() string {
	return c.String()
}

// LogValue implements [slog.LogValuer] with the fields a log may carry: the
// identifiers, kind and product, and never the title, body or address.
func (c Case) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", c.ID),
		slog.String("reference", c.Reference),
		slog.String("kind", string(c.Kind)),
		slog.String("product", c.Product),
	)
}

// Acknowledgment is the first message to the requester, rendered by the host
// from the application's template. A desk sends it as given.
type Acknowledgment struct {
	Subject string
	Body    string // Markdown; never contains a visitor's text
}

// Context is shown to responders only: for example the plan, the application
// version or the organization's name. It never reaches the requester.
type Context map[string]string

// Delivery records where a case was delivered.
type Delivery struct {
	Desk     string // the desk's name
	TicketID string // the desk's own identifier
}

// Status is a ticket's state at the desk.
type Status string

const (
	StatusOpen    Status = "open"    // waiting for a responder
	StatusPending Status = "pending" // waiting for the requester
	StatusClosed  Status = "closed"
)

// Valid reports whether s is one of the statuses above.
func (s Status) Valid() bool {
	switch s {
	case StatusOpen, StatusPending, StatusClosed:
		return true
	}
	return false
}

// Author is who wrote a message in a ticket's transcript.
type Author string

const (
	AuthorRequester Author = "requester"
	AuthorResponder Author = "responder"
	AuthorNote      Author = "note" // a responder's note, not sent to the requester
)

// Message is one entry in a ticket's transcript.
type Message struct {
	Author Author
	Body   string
	SentAt time.Time
}

// Transcript is a ticket's conversation as the desk holds it, returned for a
// requester's data export.
type Transcript struct {
	Delivery Delivery
	Messages []Message
}
