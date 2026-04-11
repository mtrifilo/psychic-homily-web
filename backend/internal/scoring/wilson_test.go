package scoring

import (
	"math"
	"testing"
)

func TestWilsonScore_NoVotes(t *testing.T) {
	score := WilsonScore(0, 0)
	if score != 0 {
		t.Errorf("expected 0, got %f", score)
	}
}

func TestWilsonScore_AllUpvotes(t *testing.T) {
	score := WilsonScore(10, 0)
	if score <= 0.7 || score >= 1.0 {
		t.Errorf("expected score in (0.7, 1.0), got %f", score)
	}
}

func TestWilsonScore_AllDownvotes(t *testing.T) {
	score := WilsonScore(0, 10)
	if score >= 0.3 || score < 0 {
		t.Errorf("expected score in [0, 0.3), got %f", score)
	}
}

func TestWilsonScore_MixedVotes(t *testing.T) {
	score := WilsonScore(8, 2)
	if score <= 0.5 || score >= 1.0 {
		t.Errorf("expected score in (0.5, 1.0), got %f", score)
	}
}

func TestWilsonScore_HighConfidence_BeatsLowSample(t *testing.T) {
	highN := WilsonScore(95, 5)
	lowN := WilsonScore(3, 0)
	if highN <= lowN {
		t.Errorf("expected high-N score (%f) > low-N score (%f)", highN, lowN)
	}
}

func TestWilsonScore_SingleUpvote(t *testing.T) {
	score := WilsonScore(1, 0)
	if score <= 0 || score >= 1.0 {
		t.Errorf("expected score in (0, 1.0), got %f", score)
	}
}

func TestWilsonScore_FiftyFifty(t *testing.T) {
	score := WilsonScore(50, 50)
	if math.Abs(score-0.5) > 0.1 {
		t.Errorf("expected score near 0.5, got %f", score)
	}
}

func TestWilsonScore_KnownValue(t *testing.T) {
	// Known computation: 10 ups, 0 downs
	// phat = 1.0, z = 1.281728756502709, n = 10
	score := WilsonScore(10, 0)
	// Pre-computed expected value (verified via Python)
	expected := 0.8588978107514387
	if math.Abs(score-expected) > 0.0001 {
		t.Errorf("expected score ~%f, got %f", expected, score)
	}
}
