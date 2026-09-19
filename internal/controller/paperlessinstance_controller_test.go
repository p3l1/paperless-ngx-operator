// SPDX-License-Identifier: AGPL-3.0-only

package controller

import (
	"bytes"
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/p3l1/paperless-ngx-operator/api/v1alpha1"
	"github.com/p3l1/paperless-ngx-operator/internal/resources"
	"github.com/p3l1/paperless-ngx-operator/test/envtestenv"
)

// testNamespace is the namespace every test in this file operates in. Each
// test starts its own envtest API server, so instances never collide across tests.
const testNamespace = "default"

// externalDBSecretName is the credentials Secret minimalInstance's external database
// points at. reconcileExternalDatabase requires it to exist and carry "username" and
// "password" keys before it reports DatabaseReady, so every test that reconciles a
// minimalInstance must create it first via createExternalDatabaseSecret.
const externalDBSecretName = "db-credentials"

// newReconciler boots a fresh envtest API server and a reconciler wired to it.
// The reconciler's scheme is its own instance, not the client's; only the type
// registrations need to match, not object identity.
func newReconciler(t *testing.T) (*PaperlessInstanceReconciler, client.Client, context.Context) {
	t.Helper()

	c, ctx := envtestenv.Start(t, v1alpha1.AddToScheme)

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("registering client-go scheme: %v", err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("registering v1alpha1 scheme: %v", err)
	}

	return &PaperlessInstanceReconciler{Client: c, Scheme: scheme}, c, ctx
}

// minimalInstance builds the smallest instance that reaches the workload step in a
// single reconcile. It points at an external database rather than leaving Database
// unset, because a managed (CNPG) database can never become ready in envtest: no
// CloudNativePG CRD is installed, and reconcileDatabase deliberately stops before
// the workload step until one is. See TestManagedDatabaseWithoutCNPGStopsBeforeWorkload
// for that path. Callers must create externalDBSecretName first (see
// createExternalDatabaseSecret): reconcileExternalDatabase now confirms it exists
// and carries valid credentials before reporting DatabaseReady.
func minimalInstance(name string) *v1alpha1.PaperlessInstance {
	return &v1alpha1.PaperlessInstance{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: v1alpha1.PaperlessInstanceSpec{
			Database: v1alpha1.DatabaseSpec{
				External: &v1alpha1.ExternalDatabase{
					Host:                 "postgres.example.org",
					CredentialsSecretRef: corev1.LocalObjectReference{Name: externalDBSecretName},
				},
			},
		},
	}
}

// createExternalDatabaseSecret creates the Secret minimalInstance's external database
// references, with the "username" and "password" keys reconcileExternalDatabase requires.
func createExternalDatabaseSecret(t *testing.T, ctx context.Context, c client.Client) {
	t.Helper()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: externalDBSecretName, Namespace: testNamespace},
		StringData: map[string]string{"username": "paperless", "password": "hunter2"},
	}
	if err := c.Create(ctx, secret); err != nil {
		t.Fatalf("creating external database secret: %v", err)
	}
}

// createMinimalInstance creates minimalInstance's backing database secret and then
// the instance itself, failing the test on any error.
func createMinimalInstance(t *testing.T, ctx context.Context, c client.Client, name string) *v1alpha1.PaperlessInstance {
	t.Helper()
	createExternalDatabaseSecret(t, ctx, c)

	inst := minimalInstance(name)
	if err := c.Create(ctx, inst); err != nil {
		t.Fatalf("creating instance: %v", err)
	}
	return inst
}

// reconcileOnce runs one Reconcile pass for inst, failing the test on error.
func reconcileOnce(t *testing.T, ctx context.Context, r *PaperlessInstanceReconciler, inst *v1alpha1.PaperlessInstance) {
	t.Helper()
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(inst)}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
}

// reconcileOnceResult is reconcileOnce but also returns the ctrl.Result, for tests
// asserting on RequeueAfter.
func reconcileOnceResult(t *testing.T, ctx context.Context, r *PaperlessInstanceReconciler, inst *v1alpha1.PaperlessInstance) ctrl.Result {
	t.Helper()
	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(inst)})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return result
}

// getInto fetches key into obj, failing the test on any error.
func getInto(t *testing.T, ctx context.Context, c client.Client, key client.ObjectKey, obj client.Object) {
	t.Helper()
	if err := c.Get(ctx, key, obj); err != nil {
		t.Fatalf("getting %T %s: %v", obj, key, err)
	}
}

// assertControlledBy fails the test unless obj carries a controller owner reference
// to owner. Ownership is how the design ties a resource's lifecycle to its instance:
// missing it means the resource would outlive (or never be swept up with) its instance.
func assertControlledBy(t *testing.T, obj client.Object, owner *v1alpha1.PaperlessInstance) {
	t.Helper()

	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID != owner.UID {
			continue
		}
		if ref.Kind != "PaperlessInstance" || ref.APIVersion != v1alpha1.GroupVersion.String() {
			t.Errorf("%T %s: owner reference has Kind=%s APIVersion=%s, want PaperlessInstance/%s",
				obj, obj.GetName(), ref.Kind, ref.APIVersion, v1alpha1.GroupVersion.String())
		}
		if ref.Controller == nil || !*ref.Controller {
			t.Errorf("%T %s: owner reference to %s is not a controller reference", obj, obj.GetName(), owner.Name)
		}
		return
	}
	t.Errorf("%T %s: no owner reference to PaperlessInstance %s (UID %s); got %+v",
		obj, obj.GetName(), owner.Name, owner.UID, obj.GetOwnerReferences())
}

// TestReconcileCreatesOwnedResourcesAndGeneratedSecrets covers the brief's first
// scenario under the default deletion policy (Retain, spec.deletionPolicy left
// unset): a minimal instance produces the Deployment, Service, four PVCs and both
// generated secrets. The Deployment and Service are always reproducible and so
// always owned; the PVCs and secrets hold state a user would mourn and so carry no
// owner reference under Retain (see TestDeletionPolicyDeleteOwnsStatefulResources
// for the same instance under DeletionPolicyDelete).
func TestReconcileCreatesOwnedResourcesAndGeneratedSecrets(t *testing.T) {
	r, c, ctx := newReconciler(t)

	inst := createMinimalInstance(t, ctx, c, "docs")
	reconcileOnce(t, ctx, r, inst)

	var fetched v1alpha1.PaperlessInstance
	getInto(t, ctx, c, client.ObjectKeyFromObject(inst), &fetched)

	var dep appsv1.Deployment
	getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: inst.Name}, &dep)
	assertControlledBy(t, &dep, &fetched)

	var svc corev1.Service
	getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: inst.Name}, &svc)
	assertControlledBy(t, &svc, &fetched)

	for _, name := range []string{
		resources.DataPVCName(inst), resources.MediaPVCName(inst),
		resources.ConsumePVCName(inst), resources.ExportPVCName(inst),
	} {
		var pvc corev1.PersistentVolumeClaim
		getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: name}, &pvc)
		if refs := pvc.GetOwnerReferences(); len(refs) != 0 {
			t.Errorf("PVC %s has owner references %+v, want none under the default Retain policy: "+
				"the document archive must survive the instance being deleted", name, refs)
		}
	}

	for _, name := range []string{resources.SecretKeyName(inst), resources.AdminSecretName(inst)} {
		var secret corev1.Secret
		getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: name}, &secret)
		if refs := secret.GetOwnerReferences(); len(refs) != 0 {
			t.Errorf("secret %s has owner references %+v, want none: a generated secret must "+
				"survive the instance being deleted and recreated over the same volumes", name, refs)
		}
	}
}

// TestReconcileReportsWorkloadNotReadyWithoutKubelet asserts the only reachable Ready
// state in envtest. Nothing schedules pods here, so a Deployment never reports a ready
// replica; Ready=True is unreachable and must not be asserted (the end-to-end test
// covers that). Asserting False/WorkloadNotReady is what's actually observable.
func TestReconcileReportsWorkloadNotReadyWithoutKubelet(t *testing.T) {
	r, c, ctx := newReconciler(t)

	inst := createMinimalInstance(t, ctx, c, "docs")
	reconcileOnce(t, ctx, r, inst)

	var fetched v1alpha1.PaperlessInstance
	getInto(t, ctx, c, client.ObjectKeyFromObject(inst), &fetched)

	cond := meta.FindStatusCondition(fetched.Status.Conditions, v1alpha1.ConditionReady)
	if cond == nil {
		t.Fatal("Ready condition missing from status.conditions")
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("Ready status = %s, want %s: envtest runs no kubelet, so no Deployment "+
			"can ever report a ready replica here", cond.Status, metav1.ConditionFalse)
	}
	if cond.Reason != "WorkloadNotReady" {
		t.Errorf("Ready reason = %q, want %q", cond.Reason, "WorkloadNotReady")
	}
}

// TestDriftCorrectionRecreatesDeletedDeployment tests the promise that a reconcile
// applies every owned object unconditionally: deleting one out from under the
// instance must not leave it deleted after the next reconcile.
func TestDriftCorrectionRecreatesDeletedDeployment(t *testing.T) {
	r, c, ctx := newReconciler(t)

	inst := createMinimalInstance(t, ctx, c, "docs")
	reconcileOnce(t, ctx, r, inst)

	key := client.ObjectKey{Namespace: testNamespace, Name: inst.Name}
	var dep appsv1.Deployment
	getInto(t, ctx, c, key, &dep)

	if err := c.Delete(ctx, &dep); err != nil {
		t.Fatalf("deleting deployment: %v", err)
	}
	if err := c.Get(ctx, key, &appsv1.Deployment{}); !apierrors.IsNotFound(err) {
		t.Fatalf("deployment still resolvable right after delete: %v", err)
	}

	reconcileOnce(t, ctx, r, inst)

	if err := c.Get(ctx, key, &dep); err != nil {
		t.Fatalf("deployment not recreated after reconcile: %v", err)
	}
}

// TestReconcileUpdatesImageTagInPlace tests that a spec change actually reaches the
// running Deployment rather than only taking effect on first creation.
func TestReconcileUpdatesImageTagInPlace(t *testing.T) {
	r, c, ctx := newReconciler(t)

	inst := createMinimalInstance(t, ctx, c, "docs")
	reconcileOnce(t, ctx, r, inst)

	var fetched v1alpha1.PaperlessInstance
	getInto(t, ctx, c, client.ObjectKeyFromObject(inst), &fetched)
	fetched.Spec.Image.Tag = "2.0.0"
	if err := c.Update(ctx, &fetched); err != nil {
		t.Fatalf("updating instance: %v", err)
	}

	reconcileOnce(t, ctx, r, &fetched)

	var dep appsv1.Deployment
	getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: inst.Name}, &dep)

	want := fetched.Spec.Image.Reference()
	if got := dep.Spec.Template.Spec.Containers[0].Image; got != want {
		t.Errorf("deployment image = %q, want %q", got, want)
	}
}

// TestUserSuppliedSecretKeyRefIsUntouchedAndGeneratesNothing covers the brief's
// scenario for spec.secretKeySecretRef: the operator must read the user's own
// secret, never write to it, and never create a generated one alongside it.
func TestUserSuppliedSecretKeyRefIsUntouchedAndGeneratesNothing(t *testing.T) {
	r, c, ctx := newReconciler(t)

	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret-key", Namespace: testNamespace},
		StringData: map[string]string{"PAPERLESS_SECRET_KEY": "user-supplied-value"},
	}
	if err := c.Create(ctx, userSecret); err != nil {
		t.Fatalf("creating user secret: %v", err)
	}
	var before corev1.Secret
	getInto(t, ctx, c, client.ObjectKeyFromObject(userSecret), &before)

	createExternalDatabaseSecret(t, ctx, c)
	inst := minimalInstance("docs")
	inst.Spec.SecretKeySecretRef = &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: userSecret.Name},
		Key:                  "PAPERLESS_SECRET_KEY",
	}
	if err := c.Create(ctx, inst); err != nil {
		t.Fatalf("creating instance: %v", err)
	}
	reconcileOnce(t, ctx, r, inst)

	var generated corev1.Secret
	err := c.Get(ctx, client.ObjectKey{Namespace: testNamespace, Name: resources.SecretKeyName(inst)}, &generated)
	if err == nil {
		t.Errorf("generated secret %s exists, want none: secretKeySecretRef was set", resources.SecretKeyName(inst))
	} else if !apierrors.IsNotFound(err) {
		t.Fatalf("checking for generated secret: %v", err)
	}

	var after corev1.Secret
	getInto(t, ctx, c, client.ObjectKeyFromObject(userSecret), &after)
	if !bytes.Equal(before.Data["PAPERLESS_SECRET_KEY"], after.Data["PAPERLESS_SECRET_KEY"]) {
		t.Error("user-supplied secret was modified by reconcile")
	}
}

// TestGeneratedSecretsAreStableAcrossReconciles is the core guarantee: SecretKey and
// AdminSecret generate a fresh random value on every call, so the reconciler must
// read an existing secret back rather than regenerate it, or every reconcile would
// rotate Django's secret key and invalidate every session.
func TestGeneratedSecretsAreStableAcrossReconciles(t *testing.T) {
	r, c, ctx := newReconciler(t)

	inst := createMinimalInstance(t, ctx, c, "docs")
	reconcileOnce(t, ctx, r, inst)

	secretKeyName := resources.SecretKeyName(inst)
	adminSecretName := resources.AdminSecretName(inst)

	var firstKey, firstAdmin corev1.Secret
	getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: secretKeyName}, &firstKey)
	getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: adminSecretName}, &firstAdmin)

	reconcileOnce(t, ctx, r, inst)

	var secondKey, secondAdmin corev1.Secret
	getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: secretKeyName}, &secondKey)
	getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: adminSecretName}, &secondAdmin)

	if !bytes.Equal(firstKey.Data["PAPERLESS_SECRET_KEY"], secondKey.Data["PAPERLESS_SECRET_KEY"]) {
		t.Error("PAPERLESS_SECRET_KEY changed across an unchanged reconcile; every existing " +
			"Django session would be invalidated")
	}
	if !bytes.Equal(firstAdmin.Data["password"], secondAdmin.Data["password"]) {
		t.Error("admin password changed across an unchanged reconcile")
	}
}

// TestGeneratedSecretsSurviveUnrelatedSpecChange goes beyond the previous test: a
// reconcile triggered by some other field changing must not regenerate either secret.
// Stability across a no-op reconcile is necessary but not sufficient; this is the
// case that would actually fire in production every time a user edits their instance.
func TestGeneratedSecretsSurviveUnrelatedSpecChange(t *testing.T) {
	r, c, ctx := newReconciler(t)

	inst := createMinimalInstance(t, ctx, c, "docs")
	reconcileOnce(t, ctx, r, inst)

	secretKeyName := resources.SecretKeyName(inst)
	adminSecretName := resources.AdminSecretName(inst)

	var beforeKey, beforeAdmin corev1.Secret
	getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: secretKeyName}, &beforeKey)
	getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: adminSecretName}, &beforeAdmin)

	var fetched v1alpha1.PaperlessInstance
	getInto(t, ctx, c, client.ObjectKeyFromObject(inst), &fetched)
	fetched.Spec.Timezone = "Europe/Berlin"
	if err := c.Update(ctx, &fetched); err != nil {
		t.Fatalf("updating instance: %v", err)
	}

	reconcileOnce(t, ctx, r, &fetched)

	var afterKey, afterAdmin corev1.Secret
	getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: secretKeyName}, &afterKey)
	getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: adminSecretName}, &afterAdmin)

	if !bytes.Equal(beforeKey.Data["PAPERLESS_SECRET_KEY"], afterKey.Data["PAPERLESS_SECRET_KEY"]) {
		t.Error("secret key rotated after an unrelated spec change")
	}
	if !bytes.Equal(beforeAdmin.Data["password"], afterAdmin.Data["password"]) {
		t.Error("admin password rotated after an unrelated spec change")
	}
}

// TestGeneratedSecretsAreNeverUpdatedOrPatched hardens the stability guarantee beyond
// comparing values: it fails immediately if the reconciler ever issues an Update or
// Patch against a Secret, not just if the stored value happens to change. The
// guarantee today rests on ensureGeneratedSecret only ever calling Get then Create;
// a future refactor that "for consistency" routed secrets through the same
// Server-Side Apply path as every other owned object would build(inst) a fresh
// random value on every call and silently reintroduce continuous key rotation,
// even though such a change could still leave the byte-comparison tests above
// passing on any run where they only inspect the two ends of a chain of applies.
func TestGeneratedSecretsAreNeverUpdatedOrPatched(t *testing.T) {
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
				t.Errorf("Update issued against Secret %s: a generated secret must only ever be Created once", obj.GetName())
			}
			return c.Update(ctx, obj, opts...)
		},
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			if _, ok := obj.(*corev1.Secret); ok {
				t.Errorf("Patch issued against Secret %s: a generated secret must only ever be Created once", obj.GetName())
			}
			return c.Patch(ctx, obj, patch, opts...)
		},
	})
	r := &PaperlessInstanceReconciler{Client: guarded, Scheme: scheme}

	inst := createMinimalInstance(t, ctx, c, "docs")
	reconcileOnce(t, ctx, r, inst)

	secretKeyName := resources.SecretKeyName(inst)
	var before corev1.Secret
	getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: secretKeyName}, &before)

	// Reconcile several more times, including one pass with an unrelated spec
	// change, to give a would-be Update or Patch every chance to fire.
	for i := 0; i < 3; i++ {
		reconcileOnce(t, ctx, r, inst)
	}

	var fetched v1alpha1.PaperlessInstance
	getInto(t, ctx, c, client.ObjectKeyFromObject(inst), &fetched)
	fetched.Spec.Timezone = "Europe/Berlin"
	if err := c.Update(ctx, &fetched); err != nil {
		t.Fatalf("updating instance: %v", err)
	}
	reconcileOnce(t, ctx, r, &fetched)

	var after corev1.Secret
	getInto(t, ctx, c, client.ObjectKey{Namespace: testNamespace, Name: secretKeyName}, &after)

	if before.ResourceVersion != after.ResourceVersion {
		t.Errorf("secret resourceVersion changed from %s to %s: something wrote to it after creation",
			before.ResourceVersion, after.ResourceVersion)
	}
	if !bytes.Equal(before.Data["PAPERLESS_SECRET_KEY"], after.Data["PAPERLESS_SECRET_KEY"]) {
		t.Error("secret key changed across reconciles")
	}
}

// TestSecondReconcileOfUnchangedInstanceIsANoop asserts the brief's idempotency
// scenario directly on the PaperlessInstance object: reconciling twice with nothing
// changed in between must leave observedGeneration matching and resourceVersion
// stable, proving the status write is only ever a no-op re-assertion of the same state.
func TestSecondReconcileOfUnchangedInstanceIsANoop(t *testing.T) {
	r, c, ctx := newReconciler(t)

	inst := createMinimalInstance(t, ctx, c, "docs")
	reconcileOnce(t, ctx, r, inst)

	var first v1alpha1.PaperlessInstance
	getInto(t, ctx, c, client.ObjectKeyFromObject(inst), &first)

	reconcileOnce(t, ctx, r, &first)

	var second v1alpha1.PaperlessInstance
	getInto(t, ctx, c, client.ObjectKeyFromObject(inst), &second)

	if second.ResourceVersion != first.ResourceVersion {
		t.Errorf("resourceVersion changed from %s to %s across an unchanged reconcile",
			first.ResourceVersion, second.ResourceVersion)
	}
	if second.Status.ObservedGeneration != second.Generation {
		t.Errorf("observedGeneration = %d, want %d (current generation)",
			second.Status.ObservedGeneration, second.Generation)
	}
}

// TestConditionObservedGenerationDistinguishesStaleFromCurrent asserts that
// status.conditions carries observedGeneration, and that the value actually moves:
// a condition computed before a spec change must report the old generation until the
// next reconcile recomputes it against the new one.
func TestConditionObservedGenerationDistinguishesStaleFromCurrent(t *testing.T) {
	r, c, ctx := newReconciler(t)

	inst := createMinimalInstance(t, ctx, c, "docs")
	reconcileOnce(t, ctx, r, inst)

	var fetched v1alpha1.PaperlessInstance
	getInto(t, ctx, c, client.ObjectKeyFromObject(inst), &fetched)
	cond := meta.FindStatusCondition(fetched.Status.Conditions, v1alpha1.ConditionReady)
	if cond == nil {
		t.Fatal("Ready condition missing from status.conditions")
	}
	if cond.ObservedGeneration != fetched.Generation {
		t.Fatalf("ObservedGeneration = %d, want %d (current generation)", cond.ObservedGeneration, fetched.Generation)
	}
	staleGeneration := cond.ObservedGeneration

	fetched.Spec.Timezone = "Europe/Berlin"
	if err := c.Update(ctx, &fetched); err != nil {
		t.Fatalf("updating instance: %v", err)
	}

	var afterSpecChange v1alpha1.PaperlessInstance
	getInto(t, ctx, c, client.ObjectKeyFromObject(inst), &afterSpecChange)
	if afterSpecChange.Generation == staleGeneration {
		t.Fatal("generation did not advance after a spec change")
	}
	staleCond := meta.FindStatusCondition(afterSpecChange.Status.Conditions, v1alpha1.ConditionReady)
	if staleCond == nil {
		t.Fatal("Ready condition disappeared after a spec change")
	}
	if staleCond.ObservedGeneration != staleGeneration {
		t.Errorf("Ready condition observedGeneration = %d before the next reconcile, want the stale value %d",
			staleCond.ObservedGeneration, staleGeneration)
	}

	reconcileOnce(t, ctx, r, &afterSpecChange)

	var current v1alpha1.PaperlessInstance
	getInto(t, ctx, c, client.ObjectKeyFromObject(inst), &current)
	currentCond := meta.FindStatusCondition(current.Status.Conditions, v1alpha1.ConditionReady)
	if currentCond == nil {
		t.Fatal("Ready condition missing after reconcile")
	}
	if currentCond.ObservedGeneration != current.Generation {
		t.Errorf("ObservedGeneration = %d after reconcile, want current generation %d",
			currentCond.ObservedGeneration, current.Generation)
	}
}

// TestManagedDatabaseWithoutCNPGStopsBeforeWorkload covers the ordering the design
// promises (see Reconcile's doc comment): an instance using the default managed
// database, with CloudNativePG not installed (the normal envtest state, and a real
// misconfiguration a user can hit), must report why and must not create a workload
// that could only ever crash-loop against a database that doesn't exist.
func TestManagedDatabaseWithoutCNPGStopsBeforeWorkload(t *testing.T) {
	r, c, ctx := newReconciler(t)

	inst := &v1alpha1.PaperlessInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "docs", Namespace: testNamespace},
		// Spec.Database is left unset: the default is a managed CNPG cluster,
		// and no CloudNativePG CRD is installed in this envtest environment.
	}
	if err := c.Create(ctx, inst); err != nil {
		t.Fatalf("creating instance: %v", err)
	}
	result := reconcileOnceResult(t, ctx, r, inst)

	// Nothing watches CustomResourceDefinitions, so without an explicit requeue this
	// instance would sit stuck until the informer's own resync, hours later.
	if result.RequeueAfter <= 0 {
		t.Errorf("RequeueAfter = %s, want a positive interval: CloudNativePG being installed "+
			"later is never otherwise observed", result.RequeueAfter)
	}

	var fetched v1alpha1.PaperlessInstance
	getInto(t, ctx, c, client.ObjectKeyFromObject(inst), &fetched)

	dbCond := meta.FindStatusCondition(fetched.Status.Conditions, v1alpha1.ConditionDatabaseReady)
	if dbCond == nil {
		t.Fatal("DatabaseReady condition missing from status.conditions")
	}
	if dbCond.Status != metav1.ConditionFalse || dbCond.Reason != "CloudNativePGMissing" {
		t.Errorf("DatabaseReady = %s/%s, want %s/%s",
			dbCond.Status, dbCond.Reason, metav1.ConditionFalse, "CloudNativePGMissing")
	}

	// Every step stamps Ready=False naming itself before status is written, so
	// Ready is always present, never True, and named for the actual blocker.
	readyCond := meta.FindStatusCondition(fetched.Status.Conditions, v1alpha1.ConditionReady)
	if readyCond == nil {
		t.Fatal("Ready condition missing from status.conditions")
	}
	if readyCond.Status != metav1.ConditionFalse || readyCond.Reason != "DatabaseNotReady" {
		t.Errorf("Ready = %s/%s, want %s/%s: the workload step must not run without a database",
			readyCond.Status, readyCond.Reason, metav1.ConditionFalse, "DatabaseNotReady")
	}

	err := c.Get(ctx, client.ObjectKey{Namespace: testNamespace, Name: inst.Name}, &appsv1.Deployment{})
	if err == nil {
		t.Error("deployment exists despite no database being ready")
	} else if !apierrors.IsNotFound(err) {
		t.Fatalf("checking for deployment: %v", err)
	}
}

// TestExternalDatabaseSecretMissingStopsBeforeWorkload covers a typo a user can
// actually make: spec.database.external.credentialsSecretRef names a Secret that
// does not exist. reconcileExternalDatabase must report why on DatabaseReady and
// stop before the workload step, the same guarantee TestManagedDatabaseWithoutCNPGStopsBeforeWorkload
// proves for the managed-database path.
func TestExternalDatabaseSecretMissingStopsBeforeWorkload(t *testing.T) {
	r, c, ctx := newReconciler(t)

	// createExternalDatabaseSecret is deliberately not called: the referenced
	// Secret must not exist for this test.
	inst := minimalInstance("docs")
	if err := c.Create(ctx, inst); err != nil {
		t.Fatalf("creating instance: %v", err)
	}
	result := reconcileOnceResult(t, ctx, r, inst)

	// Secrets are deliberately un-cached and never watched, so without an explicit
	// requeue this instance would sit stuck until the informer's own resync.
	if result.RequeueAfter <= 0 {
		t.Errorf("RequeueAfter = %s, want a positive interval: the secret being created "+
			"later is never otherwise observed", result.RequeueAfter)
	}

	var fetched v1alpha1.PaperlessInstance
	getInto(t, ctx, c, client.ObjectKeyFromObject(inst), &fetched)

	dbCond := meta.FindStatusCondition(fetched.Status.Conditions, v1alpha1.ConditionDatabaseReady)
	if dbCond == nil {
		t.Fatal("DatabaseReady condition missing from status.conditions")
	}
	if dbCond.Status != metav1.ConditionFalse || dbCond.Reason != "ExternalDatabaseSecretMissing" {
		t.Errorf("DatabaseReady = %s/%s, want %s/%s",
			dbCond.Status, dbCond.Reason, metav1.ConditionFalse, "ExternalDatabaseSecretMissing")
	}

	readyCond := meta.FindStatusCondition(fetched.Status.Conditions, v1alpha1.ConditionReady)
	if readyCond == nil {
		t.Fatal("Ready condition missing from status.conditions")
	}
	if readyCond.Status != metav1.ConditionFalse || readyCond.Reason != "DatabaseNotReady" {
		t.Errorf("Ready = %s/%s, want %s/%s: the workload step must not run without a database",
			readyCond.Status, readyCond.Reason, metav1.ConditionFalse, "DatabaseNotReady")
	}

	err := c.Get(ctx, client.ObjectKey{Namespace: testNamespace, Name: inst.Name}, &appsv1.Deployment{})
	if err == nil {
		t.Error("deployment exists despite the database credentials secret being missing")
	} else if !apierrors.IsNotFound(err) {
		t.Fatalf("checking for deployment: %v", err)
	}
}
