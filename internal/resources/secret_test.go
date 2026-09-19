// SPDX-License-Identifier: AGPL-3.0-only

package resources

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/p3l1/paperless-ngx-operator/api/v1alpha1"
)

func instance() *v1alpha1.PaperlessInstance {
	return &v1alpha1.PaperlessInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "docs", Namespace: "paperless"},
	}
}

func TestRandomKeyIsLongAndUnique(t *testing.T) {
	a, err := RandomKey(50)
	if err != nil {
		t.Fatalf("RandomKey: %v", err)
	}
	b, _ := RandomKey(50)

	if len(a) < 50 {
		t.Errorf("key length = %d, want at least 50", len(a))
	}
	if a == b {
		t.Error("two calls produced the same key")
	}
}

func TestSecretKeySecretCarriesInstanceName(t *testing.T) {
	s, err := SecretKey(instance())
	if err != nil {
		t.Fatalf("SecretKey: %v", err)
	}

	if got, want := s.Name, "docs-secret-key"; got != want {
		t.Errorf("name = %q, want %q", got, want)
	}
	if got, want := s.Namespace, "paperless"; got != want {
		t.Errorf("namespace = %q, want %q", got, want)
	}
	if v := s.StringData["PAPERLESS_SECRET_KEY"]; len(v) < 50 {
		t.Errorf("secret key is %d chars, want at least 50", len(v))
	}
}

// A generated secret must survive its instance: recreating the instance over the
// same volumes has to find the same key, or every session breaks.
func TestGeneratedSecretsAreNotOwnerReferenced(t *testing.T) {
	s, _ := SecretKey(instance())

	if len(s.OwnerReferences) != 0 {
		t.Errorf("generated secret carries %d owner references, want 0", len(s.OwnerReferences))
	}
}

func TestAdminSecretHasUsernameAndPassword(t *testing.T) {
	inst := instance()
	inst.Spec.Admin.Username = "operator"

	s, err := AdminSecret(inst)
	if err != nil {
		t.Fatalf("AdminSecret: %v", err)
	}

	if got, want := s.StringData["username"], "operator"; got != want {
		t.Errorf("username = %q, want %q", got, want)
	}
	if p := s.StringData["password"]; len(p) < 24 {
		t.Errorf("password is %d chars, want at least 24", len(p))
	}
	if strings.ContainsAny(s.StringData["password"], " \t\n") {
		t.Error("password contains whitespace, which breaks env-var delivery")
	}
}
