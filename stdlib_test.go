package supportcore_test

import (
	"os"
	"strings"
	"testing"
)

// TestStandardLibraryOnly keeps the module's promise to desk adapters: it
// adds no dependency to whatever module holds them.
func TestStandardLibraryOnly(t *testing.T) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "require") {
			t.Fatalf("go.mod requires a module: %s", line)
		}
	}
}
