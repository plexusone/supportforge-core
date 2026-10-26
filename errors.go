package supportcore

import (
	"errors"
	"time"
)

// The error classes. A desk wraps one of them in every error it returns, and
// the host's worker decides what to do next from the class alone.
var (
	// ErrRetryable: try the same desk again later. Use [Retry] to say when.
	ErrRetryable = errors.New("supportcore: retryable")
	// ErrCredential: the desk's credential is missing, expired or lacks a
	// permission. The host alerts its operator and moves to the next desk.
	ErrCredential = errors.New("supportcore: credential")
	// ErrRejected: this case cannot be delivered to this desk, for example
	// because it has no reply address. The host moves to the next desk.
	ErrRejected = errors.New("supportcore: rejected")
	// ErrUnsupported: the desk does not offer the operation.
	ErrUnsupported = errors.New("supportcore: unsupported")
)

// classes is the order [Classify] checks, from the most to the least
// specific action.
var classes = []error{ErrCredential, ErrRejected, ErrUnsupported, ErrRetryable}

// Classify returns the class err belongs to, or nil if it belongs to none.
// When err wraps more than one class, the first of ErrCredential,
// ErrRejected, ErrUnsupported and ErrRetryable wins.
func Classify(err error) error {
	for _, class := range classes {
		if errors.Is(err, class) {
			return class
		}
	}
	return nil
}

// RetryError is a retryable error with the time the desk asked the host to
// wait, such as a vendor's Retry-After.
type RetryError struct {
	Err   error         // the cause; may be nil
	After time.Duration // zero leaves the delay to the host's backoff
}

// Retry classifies err as retryable after the given delay.
func Retry(err error, after time.Duration) error {
	return &RetryError{Err: err, After: after}
}

func (e *RetryError) Error() string {
	if e.Err == nil {
		return ErrRetryable.Error()
	}
	return ErrRetryable.Error() + ": " + e.Err.Error()
}

func (e *RetryError) Unwrap() []error {
	if e.Err == nil {
		return []error{ErrRetryable}
	}
	return []error{ErrRetryable, e.Err}
}

// RetryAfter returns the delay a [RetryError] in err's chain asked for. It
// reports false when there is none or the delay is zero.
func RetryAfter(err error) (time.Duration, bool) {
	var re *RetryError
	if errors.As(err, &re) && re.After > 0 {
		return re.After, true
	}
	return 0, false
}
