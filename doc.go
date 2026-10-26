// Package supportcore defines a support request, the help desk it is
// delivered to, and how a desk reports failure.
//
// A desk adapter imports only this module, which depends on the standard
// library alone, so an adapter adds no database or mail dependency to the
// module that holds it. Intake, storage, retries and retention belong to the
// host, such as supportforge, which calls [Desk.Deliver] from its worker.
//
// The desktest package checks that a desk keeps the contract, and memdesk is
// an in-memory desk for tests.
package supportcore
