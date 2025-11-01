package security

import (
	"sync"
	"time"
)

// RateLimiter manages rate limiting for clipboard endpoint
var RateLimiter sync.Map // map[string][]time.Time - IP -> request timestamps

// CheckRateLimit checks if the IP has exceeded rate limit (20 requests per minute)
func CheckRateLimit(ip string) bool {
	now := time.Now()
	oneMinuteAgo := now.Add(-time.Minute)

	// Get or create timestamp list for this IP
	value, _ := RateLimiter.LoadOrStore(ip, []time.Time{})
	timestamps := value.([]time.Time)

	// Filter out timestamps older than 1 minute
	var recentRequests []time.Time
	for _, ts := range timestamps {
		if ts.After(oneMinuteAgo) {
			recentRequests = append(recentRequests, ts)
		}
	}

	// Check if rate limit exceeded
	if len(recentRequests) >= 20 {
		return false // rate limit exceeded
	}

	// Add current request timestamp
	recentRequests = append(recentRequests, now)
	RateLimiter.Store(ip, recentRequests)

	return true // request allowed
}
