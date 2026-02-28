package clock_test

import (
	"testing"
	"time"

	"github.com/jensholdgaard/discord-dkp-bot/internal/clock"
)

func TestReal_Now(t *testing.T) {
	clk := clock.Real{}
	before := time.Now()
	got := clk.Now()
	after := time.Now()

	if got.Before(before) || got.After(after) {
		t.Errorf("Real.Now() = %v, expected between %v and %v", got, before, after)
	}
}

func TestMock_Now(t *testing.T) {
	fixed := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.Mock{T: fixed}

	got := clk.Now()
	if !got.Equal(fixed) {
		t.Errorf("Mock.Now() = %v, want %v", got, fixed)
	}

	// Call again to ensure determinism.
	got2 := clk.Now()
	if !got2.Equal(fixed) {
		t.Errorf("Mock.Now() second call = %v, want %v", got2, fixed)
	}
}
