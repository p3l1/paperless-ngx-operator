// SPDX-License-Identifier: AGPL-3.0-only

package resources

import (
	"regexp"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

// safeKeyChars is the charset RandomKey must draw from: letters, digits, '-' and
// '_'. Anything outside it risks breaking shell or environment-variable delivery.
var safeKeyChars = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func TestRandomKeyUsesEnvSafeAlphabet(t *testing.T) {
	for i := 0; i < 20; i++ {
		key, err := RandomKey(64)
		if err != nil {
			t.Fatalf("RandomKey: %v", err)
		}
		if !safeKeyChars.MatchString(key) {
			t.Fatalf("key %q contains characters outside the safe alphabet", key)
		}
	}
}

func TestSecretKeyReturnsNilWhenUserSuppliesRef(t *testing.T) {
	inst := instance()
	inst.Spec.SecretKeySecretRef = &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
		Key:                  "PAPERLESS_SECRET_KEY",
	}

	s, err := SecretKey(inst)
	if err != nil {
		t.Fatalf("SecretKey: %v", err)
	}
	if s != nil {
		t.Errorf("SecretKey = %+v, want nil: a user-supplied secret must never be generated", s)
	}
}

func TestAdminSecretReturnsNilWhenUserSuppliesRef(t *testing.T) {
	inst := instance()
	inst.Spec.Admin.PasswordSecretRef = &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "my-admin"},
		Key:                  "password",
	}

	s, err := AdminSecret(inst)
	if err != nil {
		t.Fatalf("AdminSecret: %v", err)
	}
	if s != nil {
		t.Errorf("AdminSecret = %+v, want nil: a user-supplied password must never be generated", s)
	}
}

func TestAdminSecretReturnsNilWhenAdminDisabled(t *testing.T) {
	inst := instance()
	disabled := false
	inst.Spec.Admin.Enabled = &disabled

	s, err := AdminSecret(inst)
	if err != nil {
		t.Fatalf("AdminSecret: %v", err)
	}
	if s != nil {
		t.Errorf("AdminSecret = %+v, want nil: no local account means nothing to store", s)
	}
}

// Both refs supplied on the same instance must not interfere with each other:
// each builder decides independently, based only on its own field.
func TestBothSecretRefsSuppliedGeneratesNeither(t *testing.T) {
	inst := instance()
	inst.Spec.SecretKeySecretRef = &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
		Key:                  "PAPERLESS_SECRET_KEY",
	}
	inst.Spec.Admin.PasswordSecretRef = &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "my-admin"},
		Key:                  "password",
	}

	secretKey, err := SecretKey(inst)
	if err != nil {
		t.Fatalf("SecretKey: %v", err)
	}
	admin, err := AdminSecret(inst)
	if err != nil {
		t.Fatalf("AdminSecret: %v", err)
	}
	if secretKey != nil || admin != nil {
		t.Errorf("SecretKey = %+v, AdminSecret = %+v, want both nil", secretKey, admin)
	}
}

func TestGeneratedSecretsCarryLabels(t *testing.T) {
	inst := instance()

	s, err := SecretKey(inst)
	if err != nil {
		t.Fatalf("SecretKey: %v", err)
	}
	if got, want := s.Labels["app.kubernetes.io/instance"], "docs"; got != want {
		t.Errorf("app.kubernetes.io/instance = %q, want %q", got, want)
	}
	if s.Labels["app.kubernetes.io/managed-by"] == "" {
		t.Error("app.kubernetes.io/managed-by label is empty")
	}

	a, err := AdminSecret(inst)
	if err != nil {
		t.Fatalf("AdminSecret: %v", err)
	}
	if got, want := a.Labels["app.kubernetes.io/instance"], "docs"; got != want {
		t.Errorf("app.kubernetes.io/instance = %q, want %q", got, want)
	}
}

// An instance name near Kubernetes' own 253-character object-name ceiling is a
// valid PaperlessInstance (the API server enforces that limit uniformly), but
// appending "-secret-key" or "-admin" can push the derived Secret name over it.
// The builder must reject this with a clear error rather than hand back a Secret
// object that is doomed to fail at apply time.
func TestSecretBuildersRejectNameTooLong(t *testing.T) {
	inst := instance()
	inst.Name = strings.Repeat("a", 250)

	if _, err := SecretKey(inst); err == nil {
		t.Error("SecretKey: want error for a name exceeding 253 characters, got nil")
	}

	if _, err := AdminSecret(inst); err == nil {
		t.Error("AdminSecret: want error for a name exceeding 253 characters, got nil")
	}
}
