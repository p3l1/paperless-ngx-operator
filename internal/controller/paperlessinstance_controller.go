// SPDX-License-Identifier: AGPL-3.0-only

// Package controller reconciles the operator's custom resources.
package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8sjson "k8s.io/apimachinery/pkg/util/json"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/p3l1/paperless-ngx-operator/api/v1alpha1"
	"github.com/p3l1/paperless-ngx-operator/internal/resources"
)

// fieldOwner is the Server-Side Apply field manager for every object this
// reconciler writes, so the operator's own applies never conflict with themselves.
const fieldOwner = "paperless-ngx-operator"

// cnpgCRDName is the CustomResourceDefinition CloudNativePG registers for the
// Cluster kind; its presence is how the operator tells whether a managed
// database can actually be created, without depending on CloudNativePG itself.
const cnpgCRDName = "clusters.postgresql.cnpg.io"

// PaperlessInstanceReconciler reconciles a PaperlessInstance object.
type PaperlessInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=paperless.p3l1.de,resources=paperlessinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=paperless.p3l1.de,resources=paperlessinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=paperless.p3l1.de,resources=paperlessinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;secrets;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

// Reconcile drives one PaperlessInstance toward the state its spec describes.
// Order is deliberate: secrets first, since every later step may reference
// them; then storage; then the database, which the workload's environment
// depends on; then the cache; then the workload itself.
func (r *PaperlessInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var inst v1alpha1.PaperlessInstance
	if err := r.Get(ctx, req.NamespacedName, &inst); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.reconcileSecrets(ctx, &inst); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling secrets: %w", err)
	}
	if err := r.reconcileStorage(ctx, &inst); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling storage: %w", err)
	}

	dbReady, err := r.reconcileDatabase(ctx, &inst)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling database: %w", err)
	}
	if !dbReady {
		// Nothing further can come up without a database; report why and stop
		// here rather than building a workload that can only crash-loop.
		return ctrl.Result{}, r.updateStatus(ctx, &inst)
	}

	if err := r.reconcileCache(ctx, &inst); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling cache: %w", err)
	}
	if err := r.reconcileWorkload(ctx, &inst); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling workload: %w", err)
	}

	return ctrl.Result{}, r.updateStatus(ctx, &inst)
}

// updateStatus stamps status.observedGeneration and writes the status
// subresource. A write whose content exactly matches what is already stored
// is a no-op at the API server, so calling this unconditionally does not
// disturb an instance's resourceVersion when nothing actually changed.
func (r *PaperlessInstanceReconciler) updateStatus(ctx context.Context, inst *v1alpha1.PaperlessInstance) error {
	inst.Status.ObservedGeneration = inst.Generation
	if err := r.Status().Update(ctx, inst); err != nil {
		return fmt.Errorf("updating status: %w", err)
	}
	return nil
}

// reconcileSecrets ensures the Django secret key and admin credentials exist,
// generating each at most once (see ensureGeneratedSecret).
func (r *PaperlessInstanceReconciler) reconcileSecrets(ctx context.Context, inst *v1alpha1.PaperlessInstance) error {
	if err := r.ensureGeneratedSecret(ctx, inst, resources.SecretKeyName(inst), resources.SecretKey); err != nil {
		return fmt.Errorf("ensuring secret key: %w", err)
	}
	if err := r.ensureGeneratedSecret(ctx, inst, resources.AdminSecretName(inst), resources.AdminSecret); err != nil {
		return fmt.Errorf("ensuring admin secret: %w", err)
	}
	return nil
}

// ensureGeneratedSecret creates the Secret build returns, but only when none
// by that name exists yet. build (resources.SecretKey or resources.AdminSecret)
// generates a fresh random value on every call, so calling it only after a Get
// confirms absence — never on every reconcile — is what keeps the value
// stable: a value already stored is read back, never regenerated. build itself
// returns (nil, nil) when the user supplied their own secret reference, in
// which case there is nothing for the operator to create.
func (r *PaperlessInstanceReconciler) ensureGeneratedSecret(
	ctx context.Context,
	inst *v1alpha1.PaperlessInstance,
	name string,
	build func(*v1alpha1.PaperlessInstance) (*corev1.Secret, error),
) error {
	existing := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: inst.Namespace}, existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("getting secret %s: %w", name, err)
	}

	secret, err := build(inst)
	if err != nil {
		return err
	}
	if secret == nil {
		return nil
	}
	if err := r.Create(ctx, secret); err != nil {
		return fmt.Errorf("creating secret %s: %w", name, err)
	}
	return nil
}

// reconcileStorage applies the four PersistentVolumeClaims Paperless needs.
func (r *PaperlessInstanceReconciler) reconcileStorage(ctx context.Context, inst *v1alpha1.PaperlessInstance) error {
	for _, pvc := range resources.PVCs(inst) {
		if err := r.applyOwned(ctx, inst, pvc); err != nil {
			return fmt.Errorf("applying PVC %s: %w", pvc.Name, err)
		}
	}
	return nil
}

// reconcileDatabase configures the instance's database and reports
// DatabaseReady. External databases are assumed reachable once configured; a
// managed database requires CloudNativePG to be installed, checked by looking
// for its Cluster CustomResourceDefinition rather than importing its API
// module. The returned bool is false only when the caller must stop before
// the workload step, because there is no database for it to use yet.
func (r *PaperlessInstanceReconciler) reconcileDatabase(ctx context.Context, inst *v1alpha1.PaperlessInstance) (bool, error) {
	if inst.Spec.Database.IsExternal() {
		SetCondition(inst, v1alpha1.ConditionDatabaseReady, metav1.ConditionTrue,
			"ExternalDatabaseConfigured", "using the externally configured database")
		return true, nil
	}

	installed, err := r.cnpgInstalled(ctx)
	if err != nil {
		return false, fmt.Errorf("checking for CloudNativePG: %w", err)
	}
	if !installed {
		SetCondition(inst, v1alpha1.ConditionDatabaseReady, metav1.ConditionFalse, "CloudNativePGMissing",
			"install CloudNativePG (https://cloudnative-pg.io) to let the operator manage this instance's "+
				"database, or configure spec.database.external to use one already running")
		return false, nil
	}

	cluster, err := resources.CNPGCluster(inst)
	if err != nil {
		return false, fmt.Errorf("building CloudNativePG cluster: %w", err)
	}
	if err := r.applyOwned(ctx, inst, cluster); err != nil {
		return false, fmt.Errorf("applying CloudNativePG cluster: %w", err)
	}

	SetCondition(inst, v1alpha1.ConditionDatabaseReady, metav1.ConditionTrue,
		"CloudNativePGClusterApplied", "the managed CloudNativePG cluster has been applied")
	return true, nil
}

// cnpgInstalled reports whether CloudNativePG's Cluster CustomResourceDefinition
// is registered on the API server.
func (r *PaperlessInstanceReconciler) cnpgInstalled(ctx context.Context) (bool, error) {
	crd := &unstructured.Unstructured{}
	crd.SetGroupVersionKind(schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"})

	err := r.Get(ctx, client.ObjectKey{Name: cnpgCRDName}, crd)
	switch {
	case err == nil:
		return true, nil
	case apierrors.IsNotFound(err):
		return false, nil
	default:
		return false, err
	}
}

// reconcileCache applies the managed Valkey Deployment, Service and PVC, or
// does nothing when the instance uses an external cache.
func (r *PaperlessInstanceReconciler) reconcileCache(ctx context.Context, inst *v1alpha1.PaperlessInstance) error {
	if dep := resources.ValkeyDeployment(inst); dep != nil {
		if err := r.applyOwned(ctx, inst, dep); err != nil {
			return fmt.Errorf("applying valkey deployment: %w", err)
		}
	}
	if svc := resources.ValkeyService(inst); svc != nil {
		if err := r.applyOwned(ctx, inst, svc); err != nil {
			return fmt.Errorf("applying valkey service: %w", err)
		}
	}
	if pvc := resources.ValkeyPVC(inst); pvc != nil {
		if err := r.applyOwned(ctx, inst, pvc); err != nil {
			return fmt.Errorf("applying valkey PVC: %w", err)
		}
	}
	return nil
}

// reconcileWorkload applies the Paperless Service and Deployment, then reports
// Ready from the Deployment's own observed status: SSA reflects the server's
// response back into dep, so no extra Get is needed to read it.
func (r *PaperlessInstanceReconciler) reconcileWorkload(ctx context.Context, inst *v1alpha1.PaperlessInstance) error {
	if err := r.applyOwned(ctx, inst, resources.Service(inst)); err != nil {
		return fmt.Errorf("applying service: %w", err)
	}

	dep := resources.Deployment(inst)
	if err := r.applyOwned(ctx, inst, dep); err != nil {
		return fmt.Errorf("applying deployment: %w", err)
	}

	status, reason, message := metav1.ConditionFalse, "WorkloadNotReady", "waiting for the Deployment to report a ready replica"
	if dep.Status.ReadyReplicas >= 1 {
		status, reason, message = metav1.ConditionTrue, "WorkloadReady", "the Deployment reports at least one ready replica"
	}
	SetCondition(inst, v1alpha1.ConditionReady, status, reason, message)
	return nil
}

// applyOwned ties obj's lifecycle to inst with a controller owner reference,
// then Server-Side Applies it. Generated secrets never go through this path:
// they carry no owner reference by design (see resources.SecretKey).
func (r *PaperlessInstanceReconciler) applyOwned(ctx context.Context, inst *v1alpha1.PaperlessInstance, obj client.Object) error {
	if err := controllerutil.SetControllerReference(inst, obj, r.Scheme); err != nil {
		return fmt.Errorf("setting owner reference: %w", err)
	}
	return r.apply(ctx, obj)
}

// apply Server-Side-Applies obj under a fixed field owner. GroupVersionKind is
// stamped on first, since apply-patch JSON has an omitempty TypeMeta. RawPatch
// stands in for the now-deprecated client.Apply, which wants a typed builder.
func (r *PaperlessInstanceReconciler) apply(ctx context.Context, obj client.Object) error {
	gvk, err := apiutil.GVKForObject(obj, r.Scheme)
	if err != nil {
		return fmt.Errorf("resolving GroupVersionKind: %w", err)
	}
	obj.GetObjectKind().SetGroupVersionKind(gvk)

	data, err := k8sjson.Marshal(obj)
	if err != nil {
		return fmt.Errorf("encoding %s %s: %w", gvk.Kind, obj.GetName(), err)
	}

	patch := client.RawPatch(types.ApplyPatchType, data)
	if err := r.Patch(ctx, obj, patch, client.FieldOwner(fieldOwner), client.ForceOwnership); err != nil {
		return fmt.Errorf("applying %s %s: %w", gvk.Kind, obj.GetName(), err)
	}
	return nil
}

// SetupWithManager registers this reconciler with mgr. The CloudNativePG
// Cluster kind is deliberately not watched here: starting a watch on a kind
// that may not exist on the API server would keep the manager from starting
// at all in a cluster where CloudNativePG is absent.
func (r *PaperlessInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PaperlessInstance{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Complete(r)
}
