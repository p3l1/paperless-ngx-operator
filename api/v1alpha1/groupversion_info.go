// SPDX-License-Identifier: AGPL-3.0-only

// Package v1alpha1 contains the paperless.p3l1.de API types.
// +kubebuilder:object:generate=true
// +groupName=paperless.p3l1.de
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "paperless.p3l1.de", Version: "v1alpha1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)
