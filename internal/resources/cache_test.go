// SPDX-License-Identifier: AGPL-3.0-only

package resources

import (
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/p3l1/paperless-ngx-operator/api/v1alpha1"
)

// wantValkeyLabels is the literal label set ValkeyDeployment and ValkeyService are
// expected to stamp on their selector and pod template for inst.
func wantValkeyLabels(inst *v1alpha1.PaperlessInstance) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "paperless-ngx",
		"app.kubernetes.io/instance":   inst.Name,
		"app.kubernetes.io/managed-by": "paperless-ngx-operator",
		"app.kubernetes.io/component":  "cache",
	}
}

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
	want := wantValkeyLabels(inst)
	if d.Spec.Selector == nil || !reflect.DeepEqual(d.Spec.Selector.MatchLabels, want) {
		t.Errorf("Selector.MatchLabels = %v, want %v", d.Spec.Selector, want)
	}
	if !reflect.DeepEqual(d.Spec.Template.Labels, want) {
		t.Errorf("Template.Labels = %v, want %v", d.Spec.Template.Labels, want)
	}
}

// A RollingUpdate default would try to start a second pod before the first releases
// the ReadWriteOnce cache volume, deadlocking every rollout.
func TestValkeyDeploymentUsesRecreateStrategy(t *testing.T) {
	inst := instance()

	if got, want := ValkeyDeployment(inst).Spec.Strategy.Type, appsv1.RecreateDeploymentStrategyType; got != want {
		t.Errorf("strategy = %q, want %q", got, want)
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

	want := wantValkeyLabels(inst)
	if !reflect.DeepEqual(s.Spec.Selector, want) {
		t.Errorf("Selector = %v, want %v", s.Spec.Selector, want)
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
