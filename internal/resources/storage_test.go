// SPDX-License-Identifier: AGPL-3.0-only

package resources

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/p3l1/paperless-ngx-operator/api/v1alpha1"
)

// pvcByName finds the PVC with the given name, failing the test if none matches.
func pvcByName(t *testing.T, pvcs []*corev1.PersistentVolumeClaim, name string) *corev1.PersistentVolumeClaim {
	t.Helper()
	for _, p := range pvcs {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("no PVC named %q among %d PVCs", name, len(pvcs))
	return nil
}

func TestPVCsCount(t *testing.T) {
	pvcs := PVCs(instance())
	if len(pvcs) != 4 {
		t.Fatalf("PVCs returned %d claims, want 4", len(pvcs))
	}
}

func TestPVCStorageClassNameUnsetIsAbsent(t *testing.T) {
	inst := instance()

	for _, p := range PVCs(inst) {
		if p.Spec.StorageClassName != nil {
			t.Errorf("%s: StorageClassName = %q, want nil (absent from the spec)", p.Name, *p.Spec.StorageClassName)
		}
	}
}

func TestPVCStorageClassNameExplicitEmptyIsPreserved(t *testing.T) {
	inst := instance()
	empty := ""
	inst.Spec.Storage.Data.StorageClassName = &empty

	p := pvcByName(t, PVCs(inst), DataPVCName(inst))

	if p.Spec.StorageClassName == nil {
		t.Fatal("StorageClassName = nil, want a non-nil pointer to an empty string")
	}
	if *p.Spec.StorageClassName != "" {
		t.Errorf("StorageClassName = %q, want empty string", *p.Spec.StorageClassName)
	}
}

func TestPVCStorageClassNameSetIsPassedThrough(t *testing.T) {
	inst := instance()
	class := "fast-ssd"
	inst.Spec.Storage.Media.StorageClassName = &class

	p := pvcByName(t, PVCs(inst), MediaPVCName(inst))

	if p.Spec.StorageClassName == nil || *p.Spec.StorageClassName != "fast-ssd" {
		t.Errorf("StorageClassName = %v, want %q", p.Spec.StorageClassName, "fast-ssd")
	}
}

func TestPVCSizes(t *testing.T) {
	inst := instance()
	inst.Spec.Storage.Media.Size = resource.MustParse("50Gi")

	pvcs := PVCs(inst)

	cases := []struct {
		name string
		want string
	}{
		{DataPVCName(inst), v1alpha1.DefaultDataSize},
		{MediaPVCName(inst), "50Gi"},
		{ConsumePVCName(inst), v1alpha1.DefaultConsumeSize},
		{ExportPVCName(inst), v1alpha1.DefaultExportSize},
	}

	for _, c := range cases {
		p := pvcByName(t, pvcs, c.name)
		got := p.Spec.Resources.Requests[corev1.ResourceStorage]
		want := resource.MustParse(c.want)
		if got.Cmp(want) != 0 {
			t.Errorf("%s: size = %s, want %s", c.name, got.String(), want.String())
		}
	}
}

// A PersistentVolumeClaim without at least one access mode is rejected outright by the
// API server, so an empty spec.storage.<volume>.accessModes must not be passed through as-is.
func TestPVCAccessModesDefaultToReadWriteOnce(t *testing.T) {
	inst := instance()

	for _, p := range PVCs(inst) {
		if len(p.Spec.AccessModes) == 0 {
			t.Errorf("%s: AccessModes is empty, want a default", p.Name)
		}
	}
}

func TestPVCAccessModesFromSpecArePassedThrough(t *testing.T) {
	inst := instance()
	inst.Spec.Storage.Consume.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}

	p := pvcByName(t, PVCs(inst), ConsumePVCName(inst))

	if len(p.Spec.AccessModes) != 1 || p.Spec.AccessModes[0] != corev1.ReadWriteMany {
		t.Errorf("AccessModes = %v, want [ReadWriteMany]", p.Spec.AccessModes)
	}
}

// An instance with no storage section at all (a bare struct, bypassing CRD defaulting) must
// still produce four usable PVCs rather than panicking or emitting a zero-size claim.
func TestPVCsWithNoStorageSectionUsesDefaults(t *testing.T) {
	inst := instance()
	inst.Spec.Storage = v1alpha1.StorageSpec{}

	pvcs := PVCs(inst)

	cases := []struct {
		name string
		want string
	}{
		{DataPVCName(inst), v1alpha1.DefaultDataSize},
		{MediaPVCName(inst), v1alpha1.DefaultMediaSize},
		{ConsumePVCName(inst), v1alpha1.DefaultConsumeSize},
		{ExportPVCName(inst), v1alpha1.DefaultExportSize},
	}
	for _, c := range cases {
		p := pvcByName(t, pvcs, c.name)
		got := p.Spec.Resources.Requests[corev1.ResourceStorage]
		want := resource.MustParse(c.want)
		if got.Cmp(want) != 0 {
			t.Errorf("%s: size = %s, want %s", c.name, got.String(), want.String())
		}
		if p.Spec.StorageClassName != nil {
			t.Errorf("%s: StorageClassName = %q, want nil", c.name, *p.Spec.StorageClassName)
		}
	}
}

func TestPVCsCarryLabelsAndNamespace(t *testing.T) {
	inst := instance()

	for _, p := range PVCs(inst) {
		if p.Namespace != inst.Namespace {
			t.Errorf("%s: namespace = %q, want %q", p.Name, p.Namespace, inst.Namespace)
		}
		if p.Labels["app.kubernetes.io/instance"] != inst.Name {
			t.Errorf("%s: app.kubernetes.io/instance = %q, want %q", p.Name, p.Labels["app.kubernetes.io/instance"], inst.Name)
		}
	}
}
