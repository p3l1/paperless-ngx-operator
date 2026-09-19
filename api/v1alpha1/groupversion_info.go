// SPDX-License-Identifier: AGPL-3.0-only

// Package v1alpha1 contains the paperless.p3l1.de API types.
// +kubebuilder:object:generate=true
// +groupName=paperless.p3l1.de
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "paperless.p3l1.de", Version: "v1alpha1"}
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

// addKnownTypes registers this package's types with scheme. controller-runtime's own
// scheme.Builder is deprecated for api packages precisely to avoid pulling
// controller-runtime into a package that should depend only on apimachinery.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, &PaperlessInstance{}, &PaperlessInstanceList{})
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}
