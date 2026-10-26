// Package desktest checks that a desk keeps the [supportcore.Desk] contract.
//
// An adapter's tests run the suite against the adapter connected to a test
// double of its vendor, such as an httptest server, which reports what it
// was sent through [Vendor]:
//
//	func TestConformance(t *testing.T) {
//		desktest.Run(t, func(t *testing.T) (supportcore.Desk, desktest.Vendor) {
//			fake := newFakeVendor(t)
//			return mydesk.New(fake.URL()), fake
//		})
//	}
package desktest

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	supportcore "github.com/plexusone/supportforge-core"
)

// Vendor is a test double of the service behind a desk.
type Vendor interface {
	// Ticket reports what the vendor holds for a delivered ticket, and false
	// when it holds none.
	Ticket(d supportcore.Delivery) (Ticket, bool)

	// Fail makes the vendor's next request fail as f describes. It reports
	// false when the double cannot simulate f.
	Fail(f Failure) bool
}

// StatusSetter is implemented by a vendor double that can change a ticket's
// status as a responder would at the desk. The status check uses it when
// present.
type StatusSetter interface {
	SetStatus(d supportcore.Delivery, s supportcore.Status) bool
}

// Ticket is what a vendor holds for one ticket, split by who can see it.
type Ticket struct {
	Sent       []string // the subject and body of every message sent to the requester
	Responders []string // everything responders can see, including notes
}

// Failure is a way a vendor fails.
type Failure int

const (
	Unavailable  Failure = iota + 1 // a server error or a dropped connection
	RateLimited                     // too many requests
	Unauthorized                    // the credential is missing, expired or lacks a permission
	Invalid                         // the vendor refuses this request as it is
)

func (f Failure) String() string {
	switch f {
	case Unavailable:
		return "unavailable"
	case RateLimited:
		return "rate limited"
	case Unauthorized:
		return "unauthorized"
	case Invalid:
		return "invalid"
	}
	return fmt.Sprintf("Failure(%d)", int(f))
}

// class is the error class a desk must report for f.
func (f Failure) class() error {
	switch f {
	case Unavailable, RateLimited:
		return supportcore.ErrRetryable
	case Unauthorized:
		return supportcore.ErrCredential
	case Invalid:
		return supportcore.ErrRejected
	}
	return nil
}

// NewFunc returns a desk connected to a fresh vendor double.
type NewFunc func(t *testing.T) (supportcore.Desk, Vendor)

// Run checks the desks newDesk returns. It calls newDesk once per check, so
// each check starts with an empty vendor. Checks of an optional extension
// the desk does not implement are skipped.
func Run(t *testing.T, newDesk NewFunc) {
	t.Helper()
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			desk, vendor := newDesk(t)
			err := c.run(t.Context(), desk, vendor)
			var skip skipError
			switch {
			case errors.As(err, &skip):
				t.Skip(string(skip))
			case err != nil:
				t.Error(err)
			}
		})
	}
}

type skipError string

func (s skipError) Error() string { return string(s) }

type check struct {
	name string
	run  func(ctx context.Context, desk supportcore.Desk, v Vendor) error
}

var checks = []check{
	{"Delivers", checkDelivers},
	{"SendsTheAcknowledgment", checkSendsAcknowledgment},
	{"SendsTheRequesterNothingElse", checkSendsNothingElse},
	{"ShowsRespondersTheCase", checkShowsResponders},
	{"SendsNothingWithoutAReplyAddress", checkNoReplyAddress},
	{"ClassifiesVendorErrors", checkClassifiesErrors},
	{"ClassifiesCancellation", checkClassifiesCancellation},
	{"ErasesTickets", checkErases},
	{"ExportsTickets", checkExports},
	{"LinksWithoutPersonalData", checkLinks},
	{"ReportsStatus", checkStatus},
	{"Replies", checkReplies},
}

// Distinct markers, so a check can tell where each part of a case went.
const (
	markTitle      = "desktest-title-3f9a"
	markBody       = "desktest-body-8c21"
	markContext    = "desktest-context-5d70"
	markAckSubject = "desktest-ack-subject-1b4e"
	markAckBody    = "desktest-ack-body-6e02"
	markReply      = "desktest-reply-0a93"
	replyTo        = "requester@example.com"
	missingTicket  = "desktest-missing-ticket"
)

func signedInCase(id string) supportcore.Case {
	return supportcore.Case{
		ID:          id,
		Reference:   "SR-" + id,
		Product:     "desktest",
		Kind:        supportcore.KindProblem,
		Title:       markTitle,
		Body:        markBody,
		Page:        "/settings",
		ReplyWanted: true,
		Requester: supportcore.Requester{
			PrincipalID:    "principal-1",
			OrganizationID: "organization-1",
			ReplyTo:        replyTo,
		},
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func visitorCase(id string) supportcore.Case {
	c := signedInCase(id)
	c.Kind = supportcore.KindQuestion
	c.Requester = supportcore.Requester{ReplyTo: "visitor@example.com"}
	return c
}

func acknowledgment() supportcore.Acknowledgment {
	return supportcore.Acknowledgment{Subject: markAckSubject, Body: markAckBody}
}

func private() supportcore.Context {
	return supportcore.Context{"plan": markContext}
}

// deliver delivers c and returns what the vendor holds for it.
func deliver(ctx context.Context, desk supportcore.Desk, v Vendor, c supportcore.Case) (supportcore.Delivery, Ticket, error) {
	d, err := desk.Deliver(ctx, c, acknowledgment(), private())
	if err != nil {
		return d, Ticket{}, fmt.Errorf("Deliver(%s): %w", c.Reference, err)
	}
	tk, ok := v.Ticket(d)
	if !ok {
		return d, Ticket{}, fmt.Errorf("Deliver(%s) returned ticket %q, which the vendor does not hold", c.Reference, d.TicketID)
	}
	return d, tk, nil
}

func containsAny(texts []string, mark string) bool {
	for _, s := range texts {
		if strings.Contains(s, mark) {
			return true
		}
	}
	return false
}

func checkDelivers(ctx context.Context, desk supportcore.Desk, v Vendor) error {
	if desk.Name() == "" {
		return errors.New("Name is empty")
	}
	d1, _, err := deliver(ctx, desk, v, signedInCase("c1"))
	if err != nil {
		return err
	}
	if d1.Desk != desk.Name() {
		return fmt.Errorf("Delivery.Desk = %q, want the desk's name %q", d1.Desk, desk.Name())
	}
	if d1.TicketID == "" {
		return errors.New("Delivery.TicketID is empty")
	}
	d2, _, err := deliver(ctx, desk, v, signedInCase("c2"))
	if err != nil {
		return err
	}
	if d2.TicketID == d1.TicketID {
		return fmt.Errorf("two cases were delivered to the same ticket %q", d1.TicketID)
	}
	return nil
}

func checkSendsAcknowledgment(ctx context.Context, desk supportcore.Desk, v Vendor) error {
	_, tk, err := deliver(ctx, desk, v, signedInCase("c1"))
	if err != nil {
		return err
	}
	for _, mark := range []string{markAckSubject, markAckBody} {
		if !containsAny(tk.Sent, mark) {
			return fmt.Errorf("the requester was not sent the acknowledgment's %q", mark)
		}
	}
	return nil
}

// checkSendsNothingElse is the rule that keeps a public form from relaying
// a visitor's text to an address of their choosing.
func checkSendsNothingElse(ctx context.Context, desk supportcore.Desk, v Vendor) error {
	var errs []error
	for _, c := range []supportcore.Case{signedInCase("c1"), visitorCase("c2")} {
		_, tk, err := deliver(ctx, desk, v, c)
		if err != nil {
			return err
		}
		for _, mark := range []string{markTitle, markBody, markContext} {
			if containsAny(tk.Sent, mark) {
				errs = append(errs, fmt.Errorf("a %s was sent %q, which the acknowledgment does not contain", c.Requester, mark))
			}
		}
	}
	return errors.Join(errs...)
}

func checkShowsResponders(ctx context.Context, desk supportcore.Desk, v Vendor) error {
	_, tk, err := deliver(ctx, desk, v, visitorCase("c1"))
	if err != nil {
		return err
	}
	var errs []error
	for _, mark := range []string{markTitle, markBody, markContext} {
		if !containsAny(tk.Responders, mark) {
			errs = append(errs, fmt.Errorf("responders cannot see %q", mark))
		}
	}
	return errors.Join(errs...)
}

func checkNoReplyAddress(ctx context.Context, desk supportcore.Desk, v Vendor) error {
	c := visitorCase("c1")
	c.Kind = supportcore.KindFeedback
	c.ReplyWanted = false
	c.Requester.ReplyTo = ""
	d, err := desk.Deliver(ctx, c, acknowledgment(), private())
	if err != nil {
		if supportcore.Classify(err) != supportcore.ErrRejected {
			return fmt.Errorf("Deliver without a reply address returned %v; want success or ErrRejected", err)
		}
		return nil
	}
	tk, ok := v.Ticket(d)
	if !ok {
		return fmt.Errorf("Deliver returned ticket %q, which the vendor does not hold", d.TicketID)
	}
	if len(tk.Sent) > 0 {
		return fmt.Errorf("%d messages were sent with no reply address", len(tk.Sent))
	}
	return nil
}

func checkClassifiesErrors(ctx context.Context, desk supportcore.Desk, v Vendor) error {
	var errs []error
	simulated := 0
	for i, f := range []Failure{Unavailable, RateLimited, Unauthorized, Invalid} {
		if !v.Fail(f) {
			continue
		}
		simulated++
		_, err := desk.Deliver(ctx, signedInCase(fmt.Sprintf("f%d", i)), acknowledgment(), private())
		switch got := supportcore.Classify(err); {
		case err == nil:
			errs = append(errs, fmt.Errorf("%s: Deliver succeeded", f))
		case got != f.class():
			errs = append(errs, fmt.Errorf("%s: Deliver returned %v, classified %v, want %v", f, err, got, f.class()))
		}
	}
	if simulated == 0 {
		return errors.New("the vendor double simulates no failure, so classification is untested")
	}
	return errors.Join(errs...)
}

func checkClassifiesCancellation(ctx context.Context, desk supportcore.Desk, _ Vendor) error {
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	_, err := desk.Deliver(ctx, signedInCase("c1"), acknowledgment(), private())
	if err == nil {
		return errors.New("Deliver succeeded with a canceled context")
	}
	if got := supportcore.Classify(err); got != supportcore.ErrRetryable {
		return fmt.Errorf("Deliver with a canceled context returned %v, classified %v, want ErrRetryable", err, got)
	}
	return nil
}

func checkErases(ctx context.Context, desk supportcore.Desk, v Vendor) error {
	eraser, ok := desk.(supportcore.TicketEraser)
	if !ok {
		return skipError("the desk does not implement TicketEraser")
	}
	d, _, err := deliver(ctx, desk, v, signedInCase("c1"))
	if err != nil {
		return err
	}
	if err := eraser.EraseTicket(ctx, d); err != nil {
		return fmt.Errorf("EraseTicket: %w", err)
	}
	if _, ok := v.Ticket(d); ok {
		return fmt.Errorf("the vendor still holds ticket %q after EraseTicket", d.TicketID)
	}
	if err := eraser.EraseTicket(ctx, d); err != nil {
		return fmt.Errorf("EraseTicket of an erased ticket: %w", err)
	}
	return nil
}

func checkExports(ctx context.Context, desk supportcore.Desk, v Vendor) error {
	exporter, ok := desk.(supportcore.TicketExporter)
	if !ok {
		return skipError("the desk does not implement TicketExporter")
	}
	d, _, err := deliver(ctx, desk, v, signedInCase("c1"))
	if err != nil {
		return err
	}
	tr, err := exporter.ExportTicket(ctx, d)
	if err != nil {
		return fmt.Errorf("ExportTicket: %w", err)
	}
	if tr.Delivery != d {
		return fmt.Errorf("Transcript.Delivery = %+v, want %+v", tr.Delivery, d)
	}
	bodies := make([]string, len(tr.Messages))
	for i, m := range tr.Messages {
		bodies[i] = m.Body
	}
	for _, mark := range []string{markBody, markAckBody} {
		if !containsAny(bodies, mark) {
			return fmt.Errorf("the transcript lacks %q", mark)
		}
	}
	return nil
}

func checkLinks(ctx context.Context, desk supportcore.Desk, v Vendor) error {
	linker, ok := desk.(supportcore.TicketLinker)
	if !ok {
		return skipError("the desk does not implement TicketLinker")
	}
	d, _, err := deliver(ctx, desk, v, signedInCase("c1"))
	if err != nil {
		return err
	}
	link, err := linker.TicketURL(ctx, d)
	if err != nil {
		return fmt.Errorf("TicketURL: %w", err)
	}
	if link == "" {
		return nil
	}
	u, err := url.Parse(link)
	if err != nil {
		return fmt.Errorf("TicketURL returned %q: %w", link, err)
	}
	if (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("TicketURL returned %q, which is not an absolute web address", link)
	}
	// An escaped address such as requester%40example.com is still the address.
	unescaped, err := url.QueryUnescape(link)
	if err != nil {
		unescaped = link // malformed escapes; check the link as it is
	}
	for _, mark := range []string{markTitle, markBody, markContext, replyTo} {
		if strings.Contains(link, mark) || strings.Contains(unescaped, mark) {
			return fmt.Errorf("TicketURL returned %q, which contains %q", link, mark)
		}
	}
	return nil
}

func checkStatus(ctx context.Context, desk supportcore.Desk, v Vendor) error {
	reporter, ok := desk.(supportcore.TicketStatuser)
	if !ok {
		return skipError("the desk does not implement TicketStatuser")
	}
	d, _, err := deliver(ctx, desk, v, signedInCase("c1"))
	if err != nil {
		return err
	}
	s, err := reporter.TicketStatus(ctx, d)
	if err != nil {
		return fmt.Errorf("TicketStatus: %w", err)
	}
	if !s.Valid() || s == supportcore.StatusClosed {
		return fmt.Errorf("TicketStatus of a new ticket = %q, want open or pending", s)
	}
	setter, ok := v.(StatusSetter)
	if !ok || !setter.SetStatus(d, supportcore.StatusClosed) {
		return nil
	}
	if s, err = reporter.TicketStatus(ctx, d); err != nil {
		return fmt.Errorf("TicketStatus after closing: %w", err)
	}
	if s != supportcore.StatusClosed {
		return fmt.Errorf("TicketStatus after closing = %q, want closed", s)
	}
	return nil
}

func checkReplies(ctx context.Context, desk supportcore.Desk, v Vendor) error {
	replier, ok := desk.(supportcore.TicketReplier)
	if !ok {
		return skipError("the desk does not implement TicketReplier")
	}
	d, _, err := deliver(ctx, desk, v, signedInCase("c1"))
	if err != nil {
		return err
	}
	if err := replier.Reply(ctx, d, signedInCase("c1"), markReply); err != nil {
		return fmt.Errorf("Reply: %w", err)
	}
	tk, ok := v.Ticket(d)
	if !ok {
		return fmt.Errorf("the vendor no longer holds ticket %q after Reply", d.TicketID)
	}
	if !containsAny(tk.Sent, markReply) {
		return errors.New("the requester was not sent the reply")
	}
	if !containsAny(tk.Responders, markReply) {
		return errors.New("responders cannot see the reply")
	}
	missing := supportcore.Delivery{Desk: desk.Name(), TicketID: missingTicket}
	if err := replier.Reply(ctx, missing, signedInCase("c2"), markReply); supportcore.Classify(err) != supportcore.ErrRejected {
		return fmt.Errorf("Reply to a ticket that does not exist returned %v; want ErrRejected", err)
	}
	return nil
}
