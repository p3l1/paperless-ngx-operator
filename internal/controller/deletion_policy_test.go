// SPDX-License-Identifier: AGPL-3.0-only

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/p3l1/paperless-ngx-operator/api/v1alpha1"
	"github.com/p3l1/paperless-ngx-operator/internal/resources"
	"github.com/p3l1/paperless-ngx-operator/test/envtestenv"
)

// statefulPVCNames returns the four PVC names spec.deletionPolicy governs for inst.
func statefulPVCNames(inst *v1alpha1.PaperlessInstance) []string {
	return []string{
		resources.DataPVCName(inst), resources.MediaPVCName(inst),
		resources.ConsumePVCName(inst), resources.ExportPVCName(inst),
	}
}

// statefulSecretNames returns the two generated Secret names spec.deletionPolicy governs.
func statefulSecretNames(inst *v1alpha1.PaperlessInstance) []string {
	return []string{resources.SecretKeyName(inst), resources.AdminSecretName(inst)}
}

// assertNoOwnerReferences fails the test if obj carries any owner reference.
func assertNoOwnerReferences(t *testing.T, obj client.Object) {
	t.Helper()
	if refs := obj.GetOwnerReferences(); len(refs) != 0 {
		t.Errorf("%T %s: owner references = %+v, want none", obj, obj.GetName(), refs)
	}
}

// TestSetExplicitOwnerReferencesForcesEmptyList proves the mechanic Retain
// depends on: encoding/json's omitempty would otherwise drop a nil owner
// reference list from the apply body entirely, which Server-Side Apply reads as
// "no opinion" rather than "remove what I previously set" (see the function's doc).
func TestSetExplicitOwnerReferencesForcesEmptyList(t *testing.T) {
	data := []byte(`{"apiVersion":"v1","kind":"PersistentVolumeClaim","metadata":{"name":"x"}}`)

	out, err := setExplicitOwnerReferences(data, nil)
	if err != nil {
		t.Fatalf("setExplicitOwnerReferences: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("decoding result: %v", err)
	}
	metadata, _ := decoded["metadata"].(map[string]interface{})
	refs, present := metadata["ownerReferences"]
	if !present {
		t.Fatal("ownerReferences key is absent from the marshaled object, want it explicitly present as []")
	}
	list, ok := refs.([]interface{})
	if !ok || len(list) != 0 {
		t.Errorf("ownerReferences = %#v, want an explicit empty list", refs)
	}
}

// TestSetExplicitOwnerReferencesIncludesGivenRefs is the other half: a non-empty
// refs argument must still round-trip into the object faithfully.
func TestSetExplicitOwnerReferencesIncludesGivenRefs(t *testing.T) {
	data := []byte(`{"apiVersion":"v1","kind":"PersistentVolumeClaim","metadata":{"name":"x"}}`)
	isController := true
	refs := []metav1.OwnerReference{{
		APIVersion: v1alpha1.GroupVersion.String(),
		Kind:       "PaperlessInstance",
		Name:       "docs",
		UID:        types.UID("abc-123"),
		Controller: &isController,
	}}

	out, err := setExplicitOwnerReferences(data, refs)
	if err != nil {
		t.Fatalf("setExplicitOwnerReferences: %v", err)
	}

	var decoded corev1.PersistentVolumeClaim
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("decoding result: %v", err)
	}
	if len(decoded.OwnerReferences) != 1 || decoded.OwnerReferences[0].Name != "docs" {
		t.Errorf("ownerReferences = %+v, want one reference named %q", decoded.OwnerReferences, "docs")
	}
}

// TestDeletionPolicyDeleteOwnsStatefulResources covers the Delete policy's half of
// the split: PVCs and generated secrets, not just the Deployment and Service,
// carry a controller owner reference and so are garbage collected with the instance.
func TestDeletionPolicyDeleteOwnsStatefulResources(t *testing.T) {
	r, c, ctx := newReconciler(t)

	createExternalDatabaseSecret(t, ctx, c)
	inst := minimalInstance("docs")
	inst.Spec.DeletionPolicy = v1alpha1.DeletionPolicyDelete
	if err := c.Create(ctx, inst); err != nil {
		t.Fatalf("creating instance: %v", err)
	}
	reconcileOnce(t, ctx, r, inst)

	var fetched v1alpha1.PaperlessInstance
	getInto(t, ctx, c, client.ObjectKeyFromObject(inst), &fetched)

	for _, name := range statefulPVCNames(inst) {
		var pvc corev1.PersistentVolumeClaim
		getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: name}, &pvc)
		assertControlledBy(t, &pvc, &fetched)
	}
	for _, name := range statefulSecretNames(inst) {
		var secret corev1.Secret
		getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: name}, &secret)
		assertControlledBy(t, &secret, &fetched)
	}
}

// TestDeletionPolicySwitchToDeleteAddsOwnership proves the forward direction of a
// live policy switch: an instance reconciled under the default Retain, then moved
// to Delete, must gain owner references on its very next reconcile, not only on a
// fresh create.
func TestDeletionPolicySwitchToDeleteAddsOwnership(t *testing.T) {
	r, c, ctx := newReconciler(t)

	inst := createMinimalInstance(t, ctx, c, "docs")
	reconcileOnce(t, ctx, r, inst)

	for _, name := range statefulPVCNames(inst) {
		var pvc corev1.PersistentVolumeClaim
		getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: name}, &pvc)
		assertNoOwnerReferences(t, &pvc)
	}

	var fetched v1alpha1.PaperlessInstance
	getInto(t, ctx, c, client.ObjectKeyFromObject(inst), &fetched)
	fetched.Spec.DeletionPolicy = v1alpha1.DeletionPolicyDelete
	if err := c.Update(ctx, &fetched); err != nil {
		t.Fatalf("updating instance: %v", err)
	}
	reconcileOnce(t, ctx, r, &fetched)

	for _, name := range statefulPVCNames(inst) {
		var pvc corev1.PersistentVolumeClaim
		getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: name}, &pvc)
		assertControlledBy(t, &pvc, &fetched)
	}
	for _, name := range statefulSecretNames(inst) {
		var secret corev1.Secret
		getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: name}, &secret)
		assertControlledBy(t, &secret, &fetched)
	}
}

// TestDeletionPolicySwitchToRetainRemovesOwnership proves the direction that
// actually depends on Server-Side Apply clearing a field it previously set (see
// setExplicitOwnerReferences): an instance reconciled under Delete, then moved to
// Retain, must lose every owner reference on its very next reconcile, confirmed
// against a real envtest API server rather than a fake client.
func TestDeletionPolicySwitchToRetainRemovesOwnership(t *testing.T) {
	r, c, ctx := newReconciler(t)

	createExternalDatabaseSecret(t, ctx, c)
	inst := minimalInstance("docs")
	inst.Spec.DeletionPolicy = v1alpha1.DeletionPolicyDelete
	if err := c.Create(ctx, inst); err != nil {
		t.Fatalf("creating instance: %v", err)
	}
	reconcileOnce(t, ctx, r, inst)

	for _, name := range statefulPVCNames(inst) {
		var pvc corev1.PersistentVolumeClaim
		getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: name}, &pvc)
		if len(pvc.GetOwnerReferences()) == 0 {
			t.Fatalf("PVC %s has no owner references right after a Delete-policy reconcile", name)
		}
	}

	var fetched v1alpha1.PaperlessInstance
	getInto(t, ctx, c, client.ObjectKeyFromObject(inst), &fetched)
	fetched.Spec.DeletionPolicy = v1alpha1.DeletionPolicyRetain
	if err := c.Update(ctx, &fetched); err != nil {
		t.Fatalf("updating instance: %v", err)
	}
	reconcileOnce(t, ctx, r, &fetched)

	for _, name := range statefulPVCNames(inst) {
		var pvc corev1.PersistentVolumeClaim
		getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: name}, &pvc)
		assertNoOwnerReferences(t, &pvc)
	}
	for _, name := range statefulSecretNames(inst) {
		var secret corev1.Secret
		getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: name}, &secret)
		assertNoOwnerReferences(t, &secret)
	}
}

// patchTopLevelKeys decodes a JSON object's top-level keys, for reporting what an
// unexpectedly broad patch touched.
func patchTopLevelKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// assertPatchTouchesOnlyOwnerReferences fails the test unless patch's raw body
// touches nothing but metadata.ownerReferences — the shape
// reconcileSecretOwnership must always produce, so Data or StringData can never
// travel through a Secret patch.
func assertPatchTouchesOnlyOwnerReferences(t *testing.T, patch client.Patch, obj client.Object, name string) {
	t.Helper()

	data, err := patch.Data(obj)
	if err != nil {
		t.Fatalf("reading patch data for secret %s: %v", name, err)
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("decoding patch for secret %s: %v", name, err)
	}
	if len(body) != 1 || body["metadata"] == nil {
		t.Errorf("patch on secret %s touches top-level fields %v, want only metadata", name, patchTopLevelKeys(body))
	}

	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(body["metadata"], &metadata); err != nil {
		t.Fatalf("decoding metadata patch for secret %s: %v", name, err)
	}
	if len(metadata) != 1 || metadata["ownerReferences"] == nil {
		t.Errorf("patch on secret %s touches metadata fields %v, want only ownerReferences", name, patchTopLevelKeys(metadata))
	}
}

// TestSecretOwnershipSwitchNeverTouchesStoredValue hardens
// TestDeletionPolicySwitchToDeleteAddsOwnership/ToRetainRemovesOwnership beyond
// comparing before/after data: it intercepts every Patch and Update issued
// against a Secret while switching policy in both directions, failing the test
// if anything but a narrow ownerReferences-only patch is ever sent — the same
// guarantee TestGeneratedSecretsAreNeverUpdatedOrPatched enforces for an
// unchanged policy, extended to cover a policy that does change.
func TestSecretOwnershipSwitchNeverTouchesStoredValue(t *testing.T) {
	c, ctx := envtestenv.Start(t, v1alpha1.AddToScheme)

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("registering client-go scheme: %v", err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("registering v1alpha1 scheme: %v", err)
	}

	guarded := interceptor.NewClient(c, interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if _, ok := obj.(*corev1.Secret); ok {
				t.Errorf("Update issued against Secret %s: ownership must only ever move through a narrow patch", obj.GetName())
			}
			return c.Update(ctx, obj, opts...)
		},
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			if secret, ok := obj.(*corev1.Secret); ok {
				assertPatchTouchesOnlyOwnerReferences(t, patch, obj, secret.Name)
			}
			return c.Patch(ctx, obj, patch, opts...)
		},
	})
	r := &PaperlessInstanceReconciler{Client: guarded, Scheme: scheme}

	createExternalDatabaseSecret(t, ctx, guarded)
	inst := minimalInstance("docs")
	if err := guarded.Create(ctx, inst); err != nil {
		t.Fatalf("creating instance: %v", err)
	}
	reconcileOnce(t, ctx, r, inst)

	secretKeyName := resources.SecretKeyName(inst)
	var before corev1.Secret
	getInto(t, ctx, guarded, client.ObjectKey{Namespace: testNamespace, Name: secretKeyName}, &before)

	// Retain -> Delete: reconcileSecretOwnership must patch in an owner reference
	// without disturbing the stored key.
	var fetched v1alpha1.PaperlessInstance
	getInto(t, ctx, guarded, client.ObjectKeyFromObject(inst), &fetched)
	fetched.Spec.DeletionPolicy = v1alpha1.DeletionPolicyDelete
	if err := guarded.Update(ctx, &fetched); err != nil {
		t.Fatalf("updating instance: %v", err)
	}
	reconcileOnce(t, ctx, r, &fetched)

	var afterOwned corev1.Secret
	getInto(t, ctx, guarded, client.ObjectKey{Namespace: testNamespace, Name: secretKeyName}, &afterOwned)
	assertControlledBy(t, &afterOwned, &fetched)
	if !bytes.Equal(before.Data["PAPERLESS_SECRET_KEY"], afterOwned.Data["PAPERLESS_SECRET_KEY"]) {
		t.Error("secret key changed when only its ownership was switched to Delete")
	}

	// Delete -> Retain: the reverse patch must clear the owner reference, again
	// without touching the stored key. Re-fetched first: the reconcile above
	// already bumped resourceVersion server-side via its own status patch.
	getInto(t, ctx, guarded, client.ObjectKeyFromObject(inst), &fetched)
	fetched.Spec.DeletionPolicy = v1alpha1.DeletionPolicyRetain
	if err := guarded.Update(ctx, &fetched); err != nil {
		t.Fatalf("updating instance: %v", err)
	}
	reconcileOnce(t, ctx, r, &fetched)

	var afterRetained corev1.Secret
	getInto(t, ctx, guarded, client.ObjectKey{Namespace: testNamespace, Name: secretKeyName}, &afterRetained)
	assertNoOwnerReferences(t, &afterRetained)
	if !bytes.Equal(before.Data["PAPERLESS_SECRET_KEY"], afterRetained.Data["PAPERLESS_SECRET_KEY"]) {
		t.Error("secret key changed when only its ownership was switched back to Retain")
	}
}
