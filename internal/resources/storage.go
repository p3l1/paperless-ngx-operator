// SPDX-License-Identifier: AGPL-3.0-only

package resources

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/p3l1/paperless-ngx-operator/api/v1alpha1"
)

// defaultAccessModes is used whenever a volume's own AccessModes is empty: the API server
// rejects a PersistentVolumeClaim with none at all, so an empty spec cannot pass through as-is.
var defaultAccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}

// buildPVC assembles one PersistentVolumeClaim. storageClassName is passed through as-is
// (nil stays nil, an explicit empty string stays empty) so callers preserve the distinction
// between "let the cluster pick its default class" and "disable dynamic provisioning".
func buildPVC(inst *v1alpha1.PaperlessInstance, name, component string, size resource.Quantity, accessModes []corev1.PersistentVolumeAccessMode, storageClassName *string) *corev1.PersistentVolumeClaim {
	if len(accessModes) == 0 {
		accessModes = defaultAccessModes
	}

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: inst.Namespace,
			Labels:    commonLabels(inst, component),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      accessModes,
			StorageClassName: storageClassName,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
			},
		},
	}
}

// volumePVC builds a PVC from a user-facing VolumeSpec, reading its size through
// SizeOrDefault rather than its Size field directly (see VolumeSpec.SizeOrDefault's doc).
func volumePVC(inst *v1alpha1.PaperlessInstance, name, component string, vol v1alpha1.VolumeSpec, defaultSize string) *corev1.PersistentVolumeClaim {
	return buildPVC(inst, name, component, vol.SizeOrDefault(defaultSize), vol.AccessModes, vol.StorageClassName)
}

// PVCs returns the four PersistentVolumeClaims Paperless needs: its own working state, the
// document archive, the watched inbox, and the exporter's output directory.
func PVCs(inst *v1alpha1.PaperlessInstance) []*corev1.PersistentVolumeClaim {
	storage := inst.Spec.Storage
	return []*corev1.PersistentVolumeClaim{
		volumePVC(inst, DataPVCName(inst), "data", storage.Data, v1alpha1.DefaultDataSize),
		volumePVC(inst, MediaPVCName(inst), "media", storage.Media, v1alpha1.DefaultMediaSize),
		volumePVC(inst, ConsumePVCName(inst), "consume", storage.Consume, v1alpha1.DefaultConsumeSize),
		volumePVC(inst, ExportPVCName(inst), "export", storage.Export, v1alpha1.DefaultExportSize),
	}
}
