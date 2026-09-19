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

func TestImageReferenceDoesNotInventATag(t *testing.T) {
	img := ImageSpec{Repository: "example.org/paperless"}

	if got, want := img.Reference(), "example.org/paperless:"; got != want {
		t.Errorf("Reference() = %q, want %q (must not fall back to \"latest\")", got, want)
	}
}

func TestCacheIsManagedDefaultsTrueWhenUnset(t *testing.T) {
	if !(CacheSpec{}).IsManaged() {
		t.Error("IsManaged() = false for an empty spec; managed is the default")
	}
}

func TestCacheIsManagedFalseWhenExplicit(t *testing.T) {
	managed := false
	if (CacheSpec{Managed: &managed}).IsManaged() {
		t.Error("IsManaged() = true despite managed: false")
	}
}

func TestAdminIsEnabledDefaultsTrueWhenUnset(t *testing.T) {
	if !(AdminSpec{}).IsEnabled() {
		t.Error("IsEnabled() = false for an empty spec; enabled is the default")
	}
}

func TestAdminIsEnabledFalseWhenExplicit(t *testing.T) {
	enabled := false
	if (AdminSpec{Enabled: &enabled}).IsEnabled() {
		t.Error("IsEnabled() = true despite enabled: false")
	}
}

func TestDeletionPolicyOrDefaultDefaultsToRetainWhenUnset(t *testing.T) {
	if got := (PaperlessInstanceSpec{}).DeletionPolicyOrDefault(); got != DeletionPolicyRetain {
		t.Errorf("DeletionPolicyOrDefault() = %q, want %q", got, DeletionPolicyRetain)
	}
}

func TestDeletionPolicyOrDefaultReturnsExplicitDelete(t *testing.T) {
	spec := PaperlessInstanceSpec{DeletionPolicy: DeletionPolicyDelete}
	if got := spec.DeletionPolicyOrDefault(); got != DeletionPolicyDelete {
		t.Errorf("DeletionPolicyOrDefault() = %q, want %q", got, DeletionPolicyDelete)
	}
}

func TestDeletionPolicyOrDefaultReturnsExplicitRetain(t *testing.T) {
	spec := PaperlessInstanceSpec{DeletionPolicy: DeletionPolicyRetain}
	if got := spec.DeletionPolicyOrDefault(); got != DeletionPolicyRetain {
		t.Errorf("DeletionPolicyOrDefault() = %q, want %q", got, DeletionPolicyRetain)
	}
}
