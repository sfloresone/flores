package dur

import "testing"

func TestSeconds(t *testing.T) {
	cases := map[string]int{"45s": 45, "2m": 120, "1h30m": 5400, "1h1m1s": 3661}
	for in, want := range cases {
		got, err := Seconds(in)
		if err != nil || got != want {
			t.Errorf("Seconds(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := Seconds("10"); err == nil {
		t.Error("missing unit must be an error")
	}
}
