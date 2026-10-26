# SupportForge Core

[![Go CI][go-ci-svg]][go-ci-url]
[![Go Lint][go-lint-svg]][go-lint-url]
[![Go SAST][go-sast-svg]][go-sast-url]
[![Docs][docs-godoc-svg]][docs-godoc-url]
[![License][license-svg]][license-url]

 [go-ci-svg]: https://github.com/plexusone/supportforge-core/actions/workflows/go-ci.yaml/badge.svg?branch=main
 [go-ci-url]: https://github.com/plexusone/supportforge-core/actions/workflows/go-ci.yaml
 [go-lint-svg]: https://github.com/plexusone/supportforge-core/actions/workflows/go-lint.yaml/badge.svg?branch=main
 [go-lint-url]: https://github.com/plexusone/supportforge-core/actions/workflows/go-lint.yaml
 [go-sast-svg]: https://github.com/plexusone/supportforge-core/actions/workflows/go-sast-codeql.yaml/badge.svg?branch=main
 [go-sast-url]: https://github.com/plexusone/supportforge-core/actions/workflows/go-sast-codeql.yaml
 [docs-godoc-svg]: https://pkg.go.dev/badge/github.com/plexusone/supportforge-core
 [docs-godoc-url]: https://pkg.go.dev/github.com/plexusone/supportforge-core
 [license-svg]: https://img.shields.io/badge/license-MIT-blue.svg
 [license-url]: https://github.com/plexusone/supportforge-core/blob/main/LICENSE

The interface between a support-request system and a help desk, in Go: the
case, the `Desk` that delivers it, the error classes a desk reports, and a
conformance suite that proves a desk keeps the contract.

It depends on the standard library only, so a desk adapter can live anywhere,
in a vendor SDK, in an application or in a private repository, without
pulling in a database or mail dependency. Intake, storage, retries and
retention are the host's job; [supportforge](https://github.com/plexusone/supportforge)
is that host.

## Packages

| Package | Holds |
| --- | --- |
| `supportcore` (module root) | `Case`, `Requester`, `Acknowledgment`, `Context`, `Delivery`, `Transcript`; the `Desk` interface and its optional `TicketEraser` and `TicketExporter` extensions; the error classes |
| `desktest` | The conformance suite every desk should pass |
| `memdesk` | An in-memory desk for tests, which passes the suite |

## Writing a desk

```go
type Desk interface {
    Name() string
    Deliver(ctx context.Context, c Case, ack Acknowledgment, private Context) (Delivery, error)
}
```

A desk keeps these rules:

- **The requester is sent the acknowledgment and nothing else.** Never the
  case's text or the private context. This keeps a public form from relaying
  a visitor's text to an address of their choosing.
- **Responders see the case's title and body and the private context.**
- **No reply address, no message.** A desk that cannot create a ticket
  without one returns `ErrRejected`, and the host tries its next desk.
- **Every error is classified** by wrapping `ErrRetryable` (use `Retry` to
  pass a vendor's retry-after), `ErrCredential`, `ErrRejected` or
  `ErrUnsupported`.
- **Deliver may be retried** for a case already delivered, so use the case's
  ID as an idempotency key where the vendor accepts one.

Implement `TicketEraser` and `TicketExporter` where the vendor allows, so a
requester's erasure and data export reach the desk.

## Testing a desk

Connect the desk to a test double of its vendor, such as an `httptest`
server, that reports what it holds through `desktest.Vendor`:

```go
func TestConformance(t *testing.T) {
    desktest.Run(t, func(t *testing.T) (supportcore.Desk, desktest.Vendor) {
        fake := newFakeVendor(t)
        return mydesk.New(fake.URL()), fake
    })
}
```

The suite checks delivery, the acknowledgment, that nothing else reaches the
requester, what responders see, cases with no reply address, error
classification, cancellation, and erasure and export where implemented.

## Stability

This module changes rarely. Before v1, one adapter written outside this
repository must pass the suite. After v1, a breaking change is a new major
version.

## License

MIT
