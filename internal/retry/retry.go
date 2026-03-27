package retry

import "time"

func Backoff(attempt int, base, max time.Duration) time.Duration {
	if attempt <= 1 {
		return base
	}

	delay := base
	for i := 1; i < attempt; i++ {
		if delay >= max/2 {
			return max
		}
		delay *= 2
	}

	if delay > max {
		return max
	}

	return delay
}
