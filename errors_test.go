package supportcore_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	supportcore "github.com/plexusone/supportforge-core"
)

func TestClassify(t *testing.T) {
	cause := errors.New("status 503")
	tests := []struct {
		name string
		err  error
		want error
	}{
		{"nil", nil, nil},
		{"unclassified", cause, nil},
		{"wrapped credential", fmt.Errorf("posthog: %w", supportcore.ErrCredential), supportcore.ErrCredential},
		{"rejected", supportcore.ErrRejected, supportcore.ErrRejected},
		{"unsupported", fmt.Errorf("export: %w", supportcore.ErrUnsupported), supportcore.ErrUnsupported},
		{"retry", supportcore.Retry(cause, time.Minute), supportcore.ErrRetryable},
		{"wrapped retry", fmt.Errorf("deliver: %w", supportcore.Retry(nil, 0)), supportcore.ErrRetryable},
		{"credential wins", supportcore.Retry(supportcore.ErrCredential, 0), supportcore.ErrCredential},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := supportcore.Classify(tt.err); got != tt.want {
				t.Errorf("Classify = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetry(t *testing.T) {
	cause := errors.New("status 429")
	err := fmt.Errorf("deliver: %w", supportcore.Retry(cause, 30*time.Second))
	if !errors.Is(err, cause) {
		t.Error("the cause is lost")
	}
	if after, ok := supportcore.RetryAfter(err); !ok || after != 30*time.Second {
		t.Errorf("RetryAfter = %v, %v", after, ok)
	}
	if want := "deliver: supportcore: retryable: status 429"; err.Error() != want {
		t.Errorf("Error = %q, want %q", err.Error(), want)
	}
	if _, ok := supportcore.RetryAfter(supportcore.Retry(nil, 0)); ok {
		t.Error("a zero delay is reported")
	}
	if _, ok := supportcore.RetryAfter(cause); ok {
		t.Error("a delay is reported for a plain error")
	}
	if got := supportcore.Retry(nil, 0).Error(); got != "supportcore: retryable" {
		t.Errorf("Error = %q", got)
	}
}
