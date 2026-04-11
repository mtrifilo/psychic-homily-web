// Package scoring provides shared scoring utilities used across services.
package scoring

import "math"

// WilsonScore computes the Wilson score lower bound for ranking.
// Uses a 90% confidence interval (z = 1.281728756502709).
// Returns 0 when there are no votes.
//
// This is the same formula used by Reddit for "Best" sort and by
// PH for artist relationships, requests, and comment voting.
func WilsonScore(upvotes, downvotes int) float64 {
	n := float64(upvotes + downvotes)
	if n == 0 {
		return 0
	}
	z := 1.281728756502709
	phat := float64(upvotes) / n
	return (phat + z*z/(2*n) - z*math.Sqrt((phat*(1-phat)+z*z/(4*n))/n)) / (1 + z*z/n)
}
