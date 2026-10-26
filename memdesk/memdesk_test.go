package memdesk_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	supportcore "github.com/plexusone/supportforge-core"
	"github.com/plexusone/supportforge-core/memdesk"
)

func testCase() supportcore.Case {
	return supportcore.Case{
		ID:        "c1",
		Reference: "SR-1",
		Kind:      supportcore.KindQuestion,
		Title:     "Export",
		Body:      "How do I export?",
		Requester: supportcore.Requester{PrincipalID: "p1", ReplyTo: "a@example.com"},
	}
}

func TestDeliver(t *testing.T) {
	d := memdesk.New("")
	if d.Name() != "memory" {
		t.Errorf("Name = %q", d.Name())
	}
	ack := supportcore.Acknowledgment{Subject: "We got it", Body: "We reply by email."}
	del, err := d.Deliver(t.Context(), testCase(), ack, supportcore.Context{"plan": "free", "app": "1.2"})
	if err != nil {
		t.Fatal(err)
	}
	tk, ok := d.Ticket(del.TicketID)
	if !ok {
		t.Fatalf("no ticket %q", del.TicketID)
	}
	if len(tk.Sent) != 1 || tk.Sent[0] != (memdesk.Email{To: "a@example.com", Subject: "We got it", Body: "We reply by email."}) {
		t.Errorf("Sent = %+v", tk.Sent)
	}
	if len(tk.Transcript) != 3 {
		t.Fatalf("Transcript = %+v", tk.Transcript)
	}
	if note := tk.Transcript[1]; note.Author != supportcore.AuthorNote || note.Body != "app: 1.2\nplan: free\n" {
		t.Errorf("note = %+v", note)
	}
}

func TestDeliverWithoutReplyAddress(t *testing.T) {
	d := memdesk.New("")
	c := testCase()
	c.Requester.ReplyTo = ""
	del, err := d.Deliver(t.Context(), c, supportcore.Acknowledgment{Subject: "s"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tk, ok := d.Ticket(del.TicketID)
	if !ok {
		t.Fatalf("no ticket %q", del.TicketID)
	}
	if len(tk.Sent) != 0 || len(tk.Transcript) != 1 {
		t.Errorf("ticket = %+v", tk)
	}
}

func TestFailNext(t *testing.T) {
	d := memdesk.New("test")
	first, second := errors.New("first"), errors.New("second")
	d.FailNext(first)
	d.FailNext(second)
	for _, want := range []error{first, second, nil} {
		_, err := d.Deliver(t.Context(), testCase(), supportcore.Acknowledgment{}, nil)
		if !errors.Is(err, want) || (want == nil && err != nil) {
			t.Errorf("Deliver = %v, want %v", err, want)
		}
	}
	if d.Len() != 1 {
		t.Errorf("Len = %d, want 1", d.Len())
	}
}

func TestEraseAndExport(t *testing.T) {
	d := memdesk.New("")
	del, err := d.Deliver(t.Context(), testCase(), supportcore.Acknowledgment{Body: "ack"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := d.ExportTicket(t.Context(), del)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Delivery != del || len(tr.Messages) != 2 || !strings.Contains(tr.Messages[0].Body, "How do I export?") {
		t.Errorf("Transcript = %+v", tr)
	}
	if err := d.EraseTicket(t.Context(), del); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExportTicket(t.Context(), del); !errors.Is(err, supportcore.ErrRejected) {
		t.Errorf("ExportTicket after erasure = %v, want ErrRejected", err)
	}
}

func TestCanceledContext(t *testing.T) {
	d := memdesk.New("")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.Deliver(ctx, testCase(), supportcore.Acknowledgment{}, nil); supportcore.Classify(err) != supportcore.ErrRetryable {
		t.Errorf("Deliver = %v, want retryable", err)
	}
	if err := d.EraseTicket(ctx, supportcore.Delivery{}); supportcore.Classify(err) != supportcore.ErrRetryable {
		t.Errorf("EraseTicket = %v, want retryable", err)
	}
	if _, err := d.ExportTicket(ctx, supportcore.Delivery{}); supportcore.Classify(err) != supportcore.ErrRetryable {
		t.Errorf("ExportTicket = %v, want retryable", err)
	}
}

func TestConcurrentDelivery(t *testing.T) {
	d := memdesk.New("")
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if _, err := d.Deliver(context.Background(), testCase(), supportcore.Acknowledgment{}, nil); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if d.Len() != 20 {
		t.Errorf("Len = %d, want 20", d.Len())
	}
}

func TestLinkStatusAndReply(t *testing.T) {
	d := memdesk.New("")
	del, err := d.Deliver(t.Context(), testCase(), supportcore.Acknowledgment{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if link, err := d.TicketURL(t.Context(), del); err != nil || link != "" {
		t.Errorf("TicketURL without a portal = %q, %v", link, err)
	}
	d.SetPortal("https://desk.example.com/t/")
	if link, err := d.TicketURL(t.Context(), del); err != nil || link != "https://desk.example.com/t/"+del.TicketID {
		t.Errorf("TicketURL = %q, %v", link, err)
	}
	if s, err := d.TicketStatus(t.Context(), del); err != nil || s != supportcore.StatusOpen {
		t.Errorf("TicketStatus = %q, %v", s, err)
	}
	if err := d.Reply(t.Context(), del, testCase(), "Use the export page."); err != nil {
		t.Fatal(err)
	}
	tk, ok := d.Ticket(del.TicketID)
	if !ok {
		t.Fatal("the ticket is gone")
	}
	if tk.Status != supportcore.StatusPending || len(tk.Sent) != 2 || tk.Sent[1].Body != "Use the export page." {
		t.Errorf("ticket after Reply = %+v", tk)
	}
	if !d.SetStatus(del.TicketID, supportcore.StatusClosed) || d.SetStatus("missing", supportcore.StatusClosed) {
		t.Error("SetStatus reported the wrong result")
	}
}

func TestReplyRejected(t *testing.T) {
	d := memdesk.New("")
	c := testCase()
	c.Requester.ReplyTo = ""
	del, err := d.Deliver(t.Context(), c, supportcore.Acknowledgment{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Reply(t.Context(), del, c, "hi"); !errors.Is(err, supportcore.ErrRejected) {
		t.Errorf("Reply without an address = %v, want ErrRejected", err)
	}
	if err := d.Reply(t.Context(), supportcore.Delivery{TicketID: "missing"}, c, "hi"); !errors.Is(err, supportcore.ErrRejected) {
		t.Errorf("Reply to a missing ticket = %v, want ErrRejected", err)
	}
	d.FailNext(supportcore.ErrCredential)
	if _, err := d.TicketStatus(t.Context(), del); !errors.Is(err, supportcore.ErrCredential) {
		t.Errorf("TicketStatus = %v, want the injected error", err)
	}
}
