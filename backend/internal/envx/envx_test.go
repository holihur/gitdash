package envx

import (
	"testing"
	"time"
)

func TestMillis(t *testing.T) {
	const key = "GITDASH_TEST_MILLIS"

	t.Setenv(key, "")
	if got := Millis(key, 200); got != 200*time.Millisecond {
		t.Fatalf("empty: got %v, want 200ms", got)
	}

	t.Setenv(key, "  50 ")
	if got := Millis(key, 200); got != 50*time.Millisecond {
		t.Fatalf("valid: got %v, want 50ms", got)
	}

	t.Setenv(key, "0")
	if got := Millis(key, 200); got != 0 {
		t.Fatalf("zero: got %v, want 0", got)
	}

	t.Setenv(key, "abc")
	if got := Millis(key, 200); got != 200*time.Millisecond {
		t.Fatalf("invalid: got %v, want fallback 200ms", got)
	}

	t.Setenv(key, "-5")
	if got := Millis(key, 200); got != 200*time.Millisecond {
		t.Fatalf("negative: got %v, want fallback 200ms", got)
	}
}
