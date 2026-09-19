// SPDX-License-Identifier: AGPL-3.0-only

package controller

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConditionsObject is any object whose status exposes a Conditions slice,
// letting SetCondition serve every CRD this operator manages rather than
// being tied to PaperlessInstance alone.
type ConditionsObject interface {
	client.Object
	GetConditions() *[]metav1.Condition
}

// SetCondition upserts a condition into obj's status. ObservedGeneration is
// stamped from obj's current generation, so a condition computed before the
// latest spec change is distinguishable from one that reflects it.
func SetCondition(obj ConditionsObject, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(obj.GetConditions(), metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: obj.GetGeneration(),
		Reason:             reason,
		Message:            message,
	})
}
