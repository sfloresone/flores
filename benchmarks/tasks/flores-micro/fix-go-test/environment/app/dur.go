// Package dur parses compact durations like "1h30m" or "45s" into seconds.
package dur

import (
	"fmt"
	"strconv"
)

// Seconds returns the number of seconds in s.
func Seconds(s string) (int, error) {
	total, num := 0, ""
	for _, r := range s {
		if r >= '0' && r <= '9' {
			num += string(r)
			continue
		}
		n, err := strconv.Atoi(num)
		if err != nil {
			return 0, fmt.Errorf("bad duration %q", s)
		}
		switch r {
		case 'h':
			total += n * 3600
		case 'm':
			total += n * 360
		case 's':
			total += n
		default:
			return 0, fmt.Errorf("bad unit %q", r)
		}
		num = ""
	}
	if num != "" {
		return 0, fmt.Errorf("missing unit in %q", s)
	}
	return total, nil
}
