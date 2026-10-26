package supportcore_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	supportcore "github.com/plexusone/supportforge-core"
)

func TestKinds(t *testing.T) {
	for _, k := range supportcore.Kinds() {
		if !k.Valid() {
			t.Errorf("%q is listed but not valid", k)
		}
	}
	for _, k := range []supportcore.Kind{"", "Question", "complaint"} {
		if k.Valid() {
			t.Errorf("%q is valid", k)
		}
	}
}

func TestVisitor(t *testing.T) {
	if !(supportcore.Requester{ReplyTo: "a@example.com"}).Visitor() {
		t.Error("a requester without a principal is not a visitor")
	}
	if (supportcore.Requester{PrincipalID: "p1"}).Visitor() {
		t.Error("a requester with a principal is a visitor")
	}
}

func TestCaseDoesNotLeakIntoLogs(t *testing.T) {
	c := supportcore.Case{
		ID:        "c1",
		Reference: "SR-7K2M",
		Product:   "app",
		Kind:      supportcore.KindProblem,
		Title:     "secret-title",
		Body:      "secret-body",
		Requester: supportcore.Requester{PrincipalID: "p1", ReplyTo: "secret@example.com"},
	}
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("delivered", "case", c, "requester", c.Requester)
	outputs := map[string]string{
		"slog": buf.String(),
		"%v":   fmt.Sprintf("%v", c),
		"%+v":  fmt.Sprintf("%+v", c),
		"%#v":  fmt.Sprintf("%#v", c),
		"%s":   fmt.Sprintf("%s", c.Requester),
	}
	for format, out := range outputs {
		for _, secret := range []string{"secret-title", "secret-body", "secret@example.com"} {
			if strings.Contains(out, secret) {
				t.Errorf("%s output contains %q: %s", format, secret, out)
			}
		}
	}
	if !strings.Contains(buf.String(), "SR-7K2M") {
		t.Errorf("slog output lacks the reference: %s", buf.String())
	}
}
