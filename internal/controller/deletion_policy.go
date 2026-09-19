// SPDX-License-Identifier: AGPL-3.0-only

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sjson "k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/p3l1/paperless-ngx-operator/api/v1alpha1"
)

// applyStateful Server-Side Applies obj, one of the resources spec.deletionPolicy
// governs (a PVC or the CloudNativePG Cluster): DeletionPolicyDelete ties it to
// inst with a controller owner reference, DeletionPolicyRetain applies it with none.
func (r *PaperlessInstanceReconciler) applyStateful(ctx context.Context, inst *v1alpha1.PaperlessInstance, obj client.Object) error {
	if inst.Spec.DeletionPolicyOrDefault() == v1alpha1.DeletionPolicyDelete {
		if err := controllerutil.SetControllerReference(inst, obj, r.Scheme); err != nil {
			return fmt.Errorf("setting owner reference: %w", err)
		}
	}
	return r.apply(ctx, obj)
}

// ownerReferencesJSON renders refs as the []interface{} form setExplicitOwnerReferences
// and reconcileSecretOwnership both need to splice into a hand-built JSON object.
func ownerReferencesJSON(refs []metav1.OwnerReference) ([]interface{}, error) {
	list := make([]interface{}, 0, len(refs))
	for _, ref := range refs {
		data, err := k8sjson.Marshal(ref)
		if err != nil {
			return nil, fmt.Errorf("encoding owner reference: %w", err)
		}
		var m map[string]interface{}
		if err := k8sjson.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("decoding owner reference: %w", err)
		}
		list = append(list, m)
	}
	return list, nil
}

// setExplicitOwnerReferences rewrites data's metadata.ownerReferences to exactly
// refs, including an explicit empty list. omitempty drops a nil/empty slice from
// the marshaled object, and Apply treats an absent field as "no opinion" rather
// than "own zero entries" — so removing a reference must be stated, not implied.
func setExplicitOwnerReferences(data []byte, refs []metav1.OwnerReference) ([]byte, error) {
	var obj map[string]interface{}
	if err := k8sjson.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("decoding object: %w", err)
	}

	metadata, _ := obj["metadata"].(map[string]interface{})
	if metadata == nil {
		metadata = map[string]interface{}{}
		obj["metadata"] = metadata
	}

	list, err := ownerReferencesJSON(refs)
	if err != nil {
		return nil, err
	}
	metadata["ownerReferences"] = list

	return k8sjson.Marshal(obj)
}

// reconcileSecretOwnership aligns an existing generated secret's owner reference
// with the current deletion policy via a JSON merge patch touching only
// metadata.ownerReferences — never Data or StringData, and never the general
// apply path, which would rebuild the secret from a value-minting builder and
// rotate it. A no-op, issuing no API call, when ownership already matches.
func (r *PaperlessInstanceReconciler) reconcileSecretOwnership(ctx context.Context, inst *v1alpha1.PaperlessInstance, secret *corev1.Secret) error {
	wantOwned := inst.Spec.DeletionPolicyOrDefault() == v1alpha1.DeletionPolicyDelete
	isOwned := isControlledBy(secret, inst)
	if wantOwned == isOwned {
		return nil
	}

	var refs []metav1.OwnerReference
	if wantOwned {
		scratch := secret.DeepCopy()
		scratch.OwnerReferences = nil
		if err := controllerutil.SetControllerReference(inst, scratch, r.Scheme); err != nil {
			return fmt.Errorf("setting owner reference: %w", err)
		}
		refs = scratch.OwnerReferences
	}

	list, err := ownerReferencesJSON(refs)
	if err != nil {
		return err
	}
	body, err := k8sjson.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{"ownerReferences": list},
	})
	if err != nil {
		return fmt.Errorf("encoding owner reference patch: %w", err)
	}

	patch := client.RawPatch(types.MergePatchType, body)
	if err := r.Patch(ctx, secret, patch, client.FieldOwner(fieldOwner)); err != nil {
		return fmt.Errorf("patching owner references on secret %s: %w", secret.Name, err)
	}
	return nil
}

// isControlledBy reports whether obj's controller owner reference points at inst.
func isControlledBy(obj client.Object, inst *v1alpha1.PaperlessInstance) bool {
	ref := metav1.GetControllerOfNoCopy(obj)
	return ref != nil && ref.UID == inst.UID
}
