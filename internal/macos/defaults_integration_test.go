//go:build integration

package macos

import "testing"

// Reads only: an absent key of the test binary's own defaults.
func TestDefaultBoolAbsent(t *testing.T) {
	if DefaultBool("kami-test-absent-key") {
		t.Error("an absent key reads as true")
	}
}
