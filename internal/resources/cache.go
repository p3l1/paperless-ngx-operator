// SPDX-License-Identifier: AGPL-3.0-only

package resources

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/p3l1/paperless-ngx-operator/api/v1alpha1"
)

// defaultValkeyImage is pinned, matching how the Paperless image itself is pinned by
// default: a floating tag would let two pods run two different Valkey versions.
const defaultValkeyImage = "valkey/valkey:8.1.10-alpine"

// valkeyPort is Valkey's (and Redis's) standard listening port.
const valkeyPort = 6379

// valkeyDataSize is fixed rather than user-configurable: unlike the four Paperless
// volumes, CacheSpec has no VolumeSpec field to size the cache from.
const valkeyDataSize = "1Gi"

// valkeyReplicas is one always: nothing in this slice makes a second Valkey replica
// coordinate with the first, so scaling this Deployment would just split the cache.
var valkeyReplicas = int32(1)

func valkeyLabels(inst *v1alpha1.PaperlessInstance) map[string]string {
	return commonLabels(inst, "cache")
}

// ValkeyDeployment returns the managed cache's Deployment, or nil when the instance uses
// an external cache (spec.cache.managed: false): building it regardless would leave a
// pod running that nothing ever points at.
func ValkeyDeployment(inst *v1alpha1.PaperlessInstance) *appsv1.Deployment {
	if !inst.Spec.Cache.IsManaged() {
		return nil
	}

	name := ValkeyName(inst)
	labels := valkeyLabels(inst)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: inst.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &valkeyReplicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			// Recreate, not the Deployment default of RollingUpdate: the cache volume
			// defaults to ReadWriteOnce, so a second pod cannot start while the first
			// still holds it and a rolling update would deadlock forever (see the
			// Paperless Deployment's own Strategy field for the same reasoning).
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "valkey",
							Image: defaultValkeyImage,
							Ports: []corev1.ContainerPort{
								{Name: "valkey", ContainerPort: valkeyPort},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "data", MountPath: "/data"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: name,
								},
							},
						},
					},
				},
			},
		},
	}
}

// ValkeyService returns the managed cache's Service, or nil when the instance uses an
// external cache.
func ValkeyService(inst *v1alpha1.PaperlessInstance) *corev1.Service {
	if !inst.Spec.Cache.IsManaged() {
		return nil
	}

	labels := valkeyLabels(inst)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ValkeyName(inst),
			Namespace: inst.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{Name: "valkey", Port: valkeyPort, TargetPort: intstr.FromInt32(valkeyPort)},
			},
		},
	}
}

// ValkeyPVC returns the managed cache's PersistentVolumeClaim, or nil when the instance
// uses an external cache. Its size is fixed and it never sets a storage class, since
// CacheSpec exposes neither as a field.
func ValkeyPVC(inst *v1alpha1.PaperlessInstance) *corev1.PersistentVolumeClaim {
	if !inst.Spec.Cache.IsManaged() {
		return nil
	}

	return buildPVC(inst, ValkeyName(inst), "cache", resource.MustParse(valkeyDataSize), nil, nil)
}
