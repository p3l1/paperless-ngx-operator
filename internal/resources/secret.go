// SPDX-License-Identifier: AGPL-3.0-only

package resources

import (
	"crypto/rand"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/p3l1/paperless-ngx-operator/api/v1alpha1"
)

// randomKeyAlphabet is the 64-character base64 URL alphabet without padding: safe
// for shells and environment-variable delivery (no whitespace, no metacharacters),
// and a power of two, so masking a random byte's low 6 bits picks uniformly from
// it with no modulo bias.
const randomKeyAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

// secretKeyLength matches Django's own get_random_secret_key(), the length
// Django's documentation and tooling treat as the standard SECRET_KEY size.
const secretKeyLength = 50

// adminPasswordLength is generous for a generated password with no user-chosen
// entropy behind it; well above any reasonable minimum-length policy.
const adminPasswordLength = 32

// RandomKey returns a cryptographically random string of exactly n characters
// drawn from randomKeyAlphabet.
func RandomKey(n int) (string, error) {
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating random key: %w", err)
	}

	out := make([]byte, n)
	for i, b := range raw {
		out[i] = randomKeyAlphabet[b&0x3F]
	}
	return string(out), nil
}

// commonLabels returns the recommended Kubernetes labels identifying an object of
// the given component as belonging to inst. Generated secrets carry no owner
// reference (see SecretKey and AdminSecret), so these labels are what makes them
// discoverable as the instance's, e.g. via `kubectl get secrets -l
// app.kubernetes.io/instance=<name>`.
func commonLabels(inst *v1alpha1.PaperlessInstance, component string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "paperless-ngx",
		"app.kubernetes.io/instance":   inst.Name,
		"app.kubernetes.io/managed-by": "paperless-ngx-operator",
		"app.kubernetes.io/component":  component,
	}
}

// validateName rejects a derived object name the API server would refuse, so the
// error surfaces here with a name and reason attached instead of as an opaque
// failure from an apply call built from a Secret builders have already returned.
func validateName(name string) error {
	if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return fmt.Errorf("object name %q is invalid: %s", name, strings.Join(errs, "; "))
	}
	return nil
}

// SecretKey returns the Secret holding Django's PAPERLESS_SECRET_KEY, freshly
// generated on every call. It returns (nil, nil) when spec.secretKeySecretRef is
// set: the user supplied their own value, which the operator reads and never
// writes.
//
// The returned Secret carries no owner reference. Deleting a PaperlessInstance and
// recreating it over the same volumes must find the same key, or every existing
// session breaks — garbage-collecting this Secret with its instance would defeat
// that on every recreate. For the same reason, callers must generate this value
// once and read it back on every later reconcile: calling this again after the
// Secret already exists replaces the key and invalidates every existing session.
func SecretKey(inst *v1alpha1.PaperlessInstance) (*corev1.Secret, error) {
	if inst.Spec.SecretKeySecretRef != nil {
		return nil, nil
	}

	name := SecretKeyName(inst)
	if err := validateName(name); err != nil {
		return nil, err
	}

	key, err := RandomKey(secretKeyLength)
	if err != nil {
		return nil, fmt.Errorf("generating secret key: %w", err)
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: inst.Namespace,
			Labels:    commonLabels(inst, "secret-key"),
		},
		StringData: map[string]string{
			"PAPERLESS_SECRET_KEY": key,
		},
	}, nil
}

// AdminSecret returns the Secret holding the local superuser's username and
// password, freshly generated on every call. It returns (nil, nil) when the admin
// account is disabled (spec.admin.enabled: false — no account, nothing to store)
// or when spec.admin.passwordSecretRef is set (the user supplied their own
// password, read and never written).
//
// Like SecretKey, this Secret carries no owner reference: regenerating the
// password on every instance recreate would lock the user out of their own
// installation.
func AdminSecret(inst *v1alpha1.PaperlessInstance) (*corev1.Secret, error) {
	if !inst.Spec.Admin.IsEnabled() || inst.Spec.Admin.PasswordSecretRef != nil {
		return nil, nil
	}

	name := AdminSecretName(inst)
	if err := validateName(name); err != nil {
		return nil, err
	}

	password, err := RandomKey(adminPasswordLength)
	if err != nil {
		return nil, fmt.Errorf("generating admin password: %w", err)
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: inst.Namespace,
			Labels:    commonLabels(inst, "admin"),
		},
		StringData: map[string]string{
			"username": inst.Spec.Admin.Username,
			"password": password,
		},
	}, nil
}
