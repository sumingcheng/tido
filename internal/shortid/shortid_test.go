package shortid

import (
	"errors"
	"testing"
)

func TestEncode(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{1, "t1"},
		{9, "t9"},
		{10, "ta"},
		{35, "tz"},
		{36, "t10"},
		{118, "t3a"},
		{1296, "t100"},
	}
	for _, c := range cases {
		if got := Encode(c.n); got != c.want {
			t.Errorf("Encode(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestEncode_PanicsOnZeroOrNegative(t *testing.T) {
	for _, n := range []int64{0, -1, -100} {
		func() {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("Encode(%d) did not panic", n)
				}
			}()
			Encode(n)
		}()
	}
}

func TestDecode_RoundTrip(t *testing.T) {
	for n := int64(1); n < 10000; n += 137 {
		got, err := Decode(Encode(n))
		if err != nil {
			t.Fatalf("Decode(Encode(%d)): %v", n, err)
		}
		if got != n {
			t.Errorf("round trip %d → %d", n, got)
		}
	}
}

func TestDecode_InvalidReturnsError(t *testing.T) {
	bad := []string{"", "x1", "t", "tA", "t-1", "t 1", "t12345678a", "T1"}
	for _, s := range bad {
		_, err := Decode(s)
		if err == nil {
			t.Errorf("Decode(%q) expected error, got nil", s)
		}
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("Decode(%q) error = %v, want wrap ErrInvalid", s, err)
		}
	}
}

func TestValid(t *testing.T) {
	good := []string{"t1", "t9", "ta", "tz", "t10", "t3a", "tabc", "t12345678"}
	for _, s := range good {
		if !Valid(s) {
			t.Errorf("Valid(%q) = false, want true", s)
		}
	}
	bad := []string{"", "x1", "t", "tA", "T1", "t-1", "t 1", "t123456789"}
	for _, s := range bad {
		if Valid(s) {
			t.Errorf("Valid(%q) = true, want false", s)
		}
	}
}
