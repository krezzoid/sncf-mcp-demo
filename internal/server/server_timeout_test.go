package server

import "testing"
import "time"

func TestNavitiaTimeoutIsSufficient(t *testing.T) {
    if navitiaTimeout < 2500 * time.Millisecond {
        t.Errorf("navitiaTimeout too short: %v, want at least 2500ms", navitiaTimeout)
    }
}