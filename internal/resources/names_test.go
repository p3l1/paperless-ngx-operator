// SPDX-License-Identifier: AGPL-3.0-only

package resources

import "testing"

func TestDerivedNames(t *testing.T) {
	inst := instance()

	cases := []struct {
		name string
		fn   func() string
		want string
	}{
		{"SecretKeyName", func() string { return SecretKeyName(inst) }, "docs-secret-key"},
		{"AdminSecretName", func() string { return AdminSecretName(inst) }, "docs-admin"},
		{"DataPVCName", func() string { return DataPVCName(inst) }, "docs-data"},
		{"MediaPVCName", func() string { return MediaPVCName(inst) }, "docs-media"},
		{"ConsumePVCName", func() string { return ConsumePVCName(inst) }, "docs-consume"},
		{"ExportPVCName", func() string { return ExportPVCName(inst) }, "docs-export"},
		{"ValkeyName", func() string { return ValkeyName(inst) }, "docs-valkey"},
		{"DBName", func() string { return DBName(inst) }, "docs-db"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.fn(); got != c.want {
				t.Errorf("%s(inst) = %q, want %q", c.name, got, c.want)
			}
		})
	}
}
