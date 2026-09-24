package server

import (
	"testing"
	"time"
)

func TestNavitiaTimeoutIsSufficient(t *testing.T) {
	expected := 8000 * time.Millisecond
	if navitiaTimeout < expected {
		t.Errorf("timeout too low: expected at least %v, got %v", expected, navitiaTimeout)
	}
}