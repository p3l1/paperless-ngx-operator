// SPDX-License-Identifier: AGPL-3.0-only

package v1alpha1

import "testing"

func TestImageReferencePrefersDigest(t *testing.T) {
	img := ImageSpec{Repository: "example.org/paperless", Tag: "3.1.3", Digest: "sha256:abc"}

	if got, want := img.Reference(), "example.org/paperless@sha256:abc"; got != want {
		t.Errorf("Reference() = %q, want %q", got, want)
	}
}

func TestImageReferenceFallsBackToTag(t *testing.T) {
	img := ImageSpec{Repository: "example.org/paperless", Tag: "3.1.3"}

	if got, want := img.Reference(), "example.org/paperless:3.1.3"; got != want {
		t.Errorf("Reference() = %q, want %q", got, want)
	}
}

func TestDatabaseModeReportsExternalWhenSet(t *testing.T) {
	spec := DatabaseSpec{External: &ExternalDatabase{Host: "db.example.org"}}

	if !spec.IsExternal() {
		t.Error("IsExternal() = false for a spec with an external reference")
	}
}

func TestDatabaseModeDefaultsToManaged(t *testing.T) {
	if (DatabaseSpec{}).IsExternal() {
		t.Error("IsExternal() = true for an empty spec; managed is the default")
	}
}
