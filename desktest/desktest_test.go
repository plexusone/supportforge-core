package desktest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	supportcore "github.com/plexusone/supportforge-core"
	"github.com/plexusone/supportforge-core/memdesk"
)

// memVendor exposes a memdesk as the vendor double. Its failures are the
// errors a correct adapter would return for each vendor failure.
type memVendor struct{ desk *memdesk.Desk }

func (v memVendor) Ticket(d supportcore.Delivery) (Ticket, bool) {
	tk, ok := v.desk.Ticket(d.TicketID)
	if !ok {
		return Ticket{}, false
	}
	var out Ticket
	for _, e := range tk.Sent {
		out.Sent = append(out.Sent, e.Subject, e.Body)
	}
	for _, m := range tk.Transcript {
		out.Responders = append(out.Responders, m.Body)
	}
	return out, true
}

func (v memVendor) Fail(f Failure) bool {
	switch f {
	case Unavailable:
		v.desk.FailNext(supportcore.Retry(errors.New("status 503"), 0))
	case RateLimited:
		v.desk.FailNext(supportcore.Retry(errors.New("status 429"), 0))
	case Unauthorized:
		v.desk.FailNext(fmt.Errorf("status 401: %w", supportcore.ErrCredential))
	case Invalid:
		v.desk.FailNext(fmt.Errorf("status 400: %w", supportcore.ErrRejected))
	default:
		return false
	}
	return true
}

func newMemDesk(*testing.T) (supportcore.Desk, Vendor) {
	d := memdesk.New("")
	return d, memVendor{d}
}

func TestMemdeskConforms(t *testing.T) {
	Run(t, newMemDesk)
}

// Broken desks, each wrapping a memdesk and breaking one rule.

type echoDesk struct{ *memdesk.Desk }

func (d echoDesk) Deliver(ctx context.Context, c supportcore.Case, ack supportcore.Acknowledgment, p supportcore.Context) (supportcore.Delivery, error) {
	ack.Body += "\n\nYou wrote: " + c.Body
	return d.Desk.Deliver(ctx, c, ack, p)
}

type leakContextDesk struct{ *memdesk.Desk }

func (d leakContextDesk) Deliver(ctx context.Context, c supportcore.Case, ack supportcore.Acknowledgment, p supportcore.Context) (supportcore.Delivery, error) {
	ack.Body += "\n\nplan: " + p["plan"]
	return d.Desk.Deliver(ctx, c, ack, p)
}

type wrongNameDesk struct{ *memdesk.Desk }

func (d wrongNameDesk) Deliver(ctx context.Context, c supportcore.Case, ack supportcore.Acknowledgment, p supportcore.Context) (supportcore.Delivery, error) {
	del, err := d.Desk.Deliver(ctx, c, ack, p)
	del.Desk = "other"
	return del, err
}

type noAckDesk struct{ *memdesk.Desk }

func (d noAckDesk) Deliver(ctx context.Context, c supportcore.Case, _ supportcore.Acknowledgment, p supportcore.Context) (supportcore.Delivery, error) {
	return d.Desk.Deliver(ctx, c, supportcore.Acknowledgment{Subject: "Thanks"}, p)
}

type hideBodyDesk struct{ *memdesk.Desk }

func (d hideBodyDesk) Deliver(ctx context.Context, c supportcore.Case, ack supportcore.Acknowledgment, p supportcore.Context) (supportcore.Delivery, error) {
	c.Body = ""
	return d.Desk.Deliver(ctx, c, ack, p)
}

// unclassifiedVendor makes the desk fail with errors that carry no class.
type unclassifiedVendor struct{ memVendor }

func (v unclassifiedVendor) Fail(Failure) bool {
	v.desk.FailNext(errors.New("status 500"))
	return true
}

type noFailVendor struct{ memVendor }

func (noFailVendor) Fail(Failure) bool { return false }

type ignoresContextDesk struct{ *memdesk.Desk }

func (d ignoresContextDesk) Deliver(_ context.Context, c supportcore.Case, ack supportcore.Acknowledgment, p supportcore.Context) (supportcore.Delivery, error) {
	return d.Desk.Deliver(context.Background(), c, ack, p)
}

// keepsTicketsDesk claims erasure but does not erase.
type keepsTicketsDesk struct{ *memdesk.Desk }

func (keepsTicketsDesk) EraseTicket(context.Context, supportcore.Delivery) error { return nil }

func TestChecksCatchBrokenDesks(t *testing.T) {
	tests := []struct {
		check   string
		newDesk func(d *memdesk.Desk) (supportcore.Desk, Vendor)
		want    string
	}{
		{"SendsTheRequesterNothingElse", func(d *memdesk.Desk) (supportcore.Desk, Vendor) { return echoDesk{d}, memVendor{d} }, markBody},
		{"SendsTheRequesterNothingElse", func(d *memdesk.Desk) (supportcore.Desk, Vendor) { return leakContextDesk{d}, memVendor{d} }, markContext},
		{"Delivers", func(d *memdesk.Desk) (supportcore.Desk, Vendor) { return wrongNameDesk{d}, memVendor{d} }, "Delivery.Desk"},
		{"SendsTheAcknowledgment", func(d *memdesk.Desk) (supportcore.Desk, Vendor) { return noAckDesk{d}, memVendor{d} }, markAckSubject},
		{"ShowsRespondersTheCase", func(d *memdesk.Desk) (supportcore.Desk, Vendor) { return hideBodyDesk{d}, memVendor{d} }, markBody},
		{"ClassifiesVendorErrors", func(d *memdesk.Desk) (supportcore.Desk, Vendor) { return d, unclassifiedVendor{memVendor{d}} }, "classified <nil>"},
		{"ClassifiesVendorErrors", func(d *memdesk.Desk) (supportcore.Desk, Vendor) { return d, noFailVendor{memVendor{d}} }, "simulates no failure"},
		{"ClassifiesCancellation", func(d *memdesk.Desk) (supportcore.Desk, Vendor) { return ignoresContextDesk{d}, memVendor{d} }, "canceled context"},
		{"ErasesTickets", func(d *memdesk.Desk) (supportcore.Desk, Vendor) { return keepsTicketsDesk{d}, memVendor{d} }, "still holds"},
	}
	for _, tt := range tests {
		t.Run(tt.check+"/"+tt.want, func(t *testing.T) {
			desk, vendor := tt.newDesk(memdesk.New(""))
			err := find(t, tt.check).run(t.Context(), desk, vendor)
			if err == nil {
				t.Fatal("the check passed a broken desk")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not mention %q", err, tt.want)
			}
		})
	}
}

func TestOptionalChecksSkip(t *testing.T) {
	// Embedding only the Desk interface hides memdesk's extensions.
	type plainDesk struct{ supportcore.Desk }
	d := memdesk.New("")
	for _, name := range []string{"ErasesTickets", "ExportsTickets"} {
		err := find(t, name).run(t.Context(), plainDesk{d}, memVendor{d})
		var skip skipError
		if !errors.As(err, &skip) {
			t.Errorf("%s: got %v, want a skip", name, err)
		}
	}
}

func find(t *testing.T, name string) check {
	t.Helper()
	for _, c := range checks {
		if c.name == name {
			return c
		}
	}
	t.Fatalf("no check %q", name)
	return check{}
}
