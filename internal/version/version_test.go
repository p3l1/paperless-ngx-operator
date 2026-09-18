// SPDX-License-Identifier: AGPL-3.0-only

package version

import "testing"

func TestStringCombinesVersionAndCommit(t *testing.T) {
	Version = "1.2.3"
	Commit = "abc1234"

	if got, want := String(), "1.2.3 (abc1234)"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestStringFallsBackToDevWhenUnset(t *testing.T) {
	Version = ""
	Commit = ""

	if got, want := String(), "dev (unknown)"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
