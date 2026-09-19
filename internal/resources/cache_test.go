// SPDX-License-Identifier: AGPL-3.0-only

package resources

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestValkeyResourcesBuiltWhenManagedIsUnset(t *testing.T) {
	inst := instance() // spec.cache.managed is nil, which IsManaged() treats as true

	if ValkeyDeployment(inst) == nil {
		t.Error("ValkeyDeployment = nil, want a Deployment when cache.managed is unset")
	}
	if ValkeyService(inst) == nil {
		t.Error("ValkeyService = nil, want a Service when cache.managed is unset")
	}
	if ValkeyPVC(inst) == nil {
		t.Error("ValkeyPVC = nil, want a PVC when cache.managed is unset")
	}
}

func TestValkeyResourcesNilWhenUnmanagedWithURL(t *testing.T) {
	inst := instance()
	managed := false
	inst.Spec.Cache.Managed = &managed
	inst.Spec.Cache.URL = "redis://cache.example.org:6379"

	if d := ValkeyDeployment(inst); d != nil {
		t.Errorf("ValkeyDeployment = %+v, want nil for an unmanaged cache", d)
	}
	if s := ValkeyService(inst); s != nil {
		t.Errorf("ValkeyService = %+v, want nil for an unmanaged cache", s)
	}
	if p := ValkeyPVC(inst); p != nil {
		t.Errorf("ValkeyPVC = %+v, want nil for an unmanaged cache", p)
	}
}

// An unmanaged cache supplying only urlSecretRef (no plain URL) must still suppress the
// managed Valkey objects: the gate is IsManaged(), never "is URL empty".
func TestValkeyResourcesNilWhenUnmanagedWithOnlyURLSecretRef(t *testing.T) {
	inst := instance()
	managed := false
	inst.Spec.Cache.Managed = &managed
	inst.Spec.Cache.URLSecretRef = &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "cache-creds"},
		Key:                  "url",
	}

	if d := ValkeyDeployment(inst); d != nil {
		t.Errorf("ValkeyDeployment = %+v, want nil for an unmanaged cache with only urlSecretRef", d)
	}
	if s := ValkeyService(inst); s != nil {
		t.Errorf("ValkeyService = %+v, want nil for an unmanaged cache with only urlSecretRef", s)
	}
	if p := ValkeyPVC(inst); p != nil {
		t.Errorf("ValkeyPVC = %+v, want nil for an unmanaged cache with only urlSecretRef", p)
	}
}

func TestValkeyDeploymentShape(t *testing.T) {
	inst := instance()

	d := ValkeyDeployment(inst)
	if d == nil {
		t.Fatal("ValkeyDeployment = nil, want a Deployment")
	}

	if got, want := d.Name, ValkeyName(inst); got != want {
		t.Errorf("name = %q, want %q", got, want)
	}
	if got, want := d.Namespace, inst.Namespace; got != want {
		t.Errorf("namespace = %q, want %q", got, want)
	}
	if len(d.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("containers = %d, want 1", len(d.Spec.Template.Spec.Containers))
	}

	c := d.Spec.Template.Spec.Containers[0]
	if len(c.Ports) != 1 || c.Ports[0].ContainerPort != 6379 {
		t.Errorf("container ports = %+v, want a single 6379", c.Ports)
	}

	found := false
	for _, v := range d.Spec.Template.Spec.Volumes {
		if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == ValkeyName(inst) {
			found = true
		}
	}
	if !found {
		t.Error("no volume references the Valkey PVC by name")
	}

	// The Deployment's pod selector must match the pods it creates, or the API server
	// rejects it and, if it somehow slipped through, the Deployment would never go Ready.
	if d.Spec.Selector == nil {
		t.Fatal("Selector is nil")
	}
	for k, v := range d.Spec.Selector.MatchLabels {
		if d.Spec.Template.Labels[k] != v {
			t.Errorf("pod template label %q = %q, selector wants %q", k, d.Spec.Template.Labels[k], v)
		}
	}
}

func TestValkeyServiceShape(t *testing.T) {
	inst := instance()

	s := ValkeyService(inst)
	if s == nil {
		t.Fatal("ValkeyService = nil, want a Service")
	}
	if got, want := s.Name, ValkeyName(inst); got != want {
		t.Errorf("name = %q, want %q", got, want)
	}
	if len(s.Spec.Ports) != 1 || s.Spec.Ports[0].Port != 6379 {
		t.Errorf("service ports = %+v, want a single 6379", s.Spec.Ports)
	}

	d := ValkeyDeployment(inst)
	for k, v := range s.Spec.Selector {
		if d.Spec.Template.Labels[k] != v {
			t.Errorf("service selector %q = %q, Deployment pod label is %q", k, v, d.Spec.Template.Labels[k])
		}
	}
}

func TestValkeyPVCShape(t *testing.T) {
	inst := instance()

	p := ValkeyPVC(inst)
	if p == nil {
		t.Fatal("ValkeyPVC = nil, want a PVC")
	}
	if got, want := p.Name, ValkeyName(inst); got != want {
		t.Errorf("name = %q, want %q", got, want)
	}
	if len(p.Spec.AccessModes) != 1 || p.Spec.AccessModes[0] != corev1.ReadWriteOnce {
		t.Errorf("AccessModes = %v, want [ReadWriteOnce]", p.Spec.AccessModes)
	}
	if p.Spec.StorageClassName != nil {
		t.Errorf("StorageClassName = %q, want nil: cache storage has no per-instance class field", *p.Spec.StorageClassName)
	}
	if _, ok := p.Spec.Resources.Requests[corev1.ResourceStorage]; !ok {
		t.Error("no storage request set")
	}
}
