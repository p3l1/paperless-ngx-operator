// SPDX-License-Identifier: AGPL-3.0-only

package resources

import (
	"fmt"
	"net/url"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/p3l1/paperless-ngx-operator/api/v1alpha1"
)

// defaultTimezone and defaultOCRLanguage are neutral defaults: the operator assumes
// nothing about the user's own timezone or preferred OCR language.
const (
	defaultTimezone    = "UTC"
	defaultOCRLanguage = "eng"
)

// defaultAdminUsername mirrors AdminSpec's own kubebuilder default, needed here too
// since a bare spec (bypassing CRD defaulting) leaves Username empty.
const defaultAdminUsername = "admin"

// paperlessPort is the port Paperless's built-in webserver listens on.
const paperlessPort = 8000

// paperlessReplicas is one always: nothing in this slice makes a second replica safe
// with the four volumes' default ReadWriteOnce access mode.
var paperlessReplicas = int32(1)

// A fresh instance's first start runs a database migration that can take minutes.
// Readiness tolerates ~5 minutes before the Service routes to the pod; liveness
// tolerates longer still, so a slow migration is never mistaken for a hang.
const (
	readinessInitialDelaySeconds int32 = 10
	readinessPeriodSeconds       int32 = 10
	readinessTimeoutSeconds      int32 = 5
	readinessFailureThreshold    int32 = 30

	livenessInitialDelaySeconds int32 = 60
	livenessPeriodSeconds       int32 = 20
	livenessTimeoutSeconds      int32 = 5
	livenessFailureThreshold    int32 = 30
)

func workloadLabels(inst *v1alpha1.PaperlessInstance) map[string]string {
	return commonLabels(inst, "server")
}

// paperlessProbe is an HTTP GET against "/". host overrides the Host header: kubelet
// dials the pod IP directly and defaults Host to that IP, which Django rejects as
// outside PAPERLESS_ALLOWED_HOSTS, failing every probe with a 400.
func paperlessProbe(host string, initialDelay, period, timeout, failureThreshold int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:        "/",
				Port:        intstr.FromInt32(paperlessPort),
				HTTPHeaders: []corev1.HTTPHeader{{Name: "Host", Value: host}},
			},
		},
		InitialDelaySeconds: initialDelay,
		PeriodSeconds:       period,
		TimeoutSeconds:      timeout,
		FailureThreshold:    failureThreshold,
	}
}

// serviceDNSNames returns the Host header values a request arrives with when it comes
// through the in-cluster Service, shortest to fully qualified.
func serviceDNSNames(inst *v1alpha1.PaperlessInstance) []string {
	return []string{
		inst.Name,
		fmt.Sprintf("%s.%s.svc", inst.Name, inst.Namespace),
		fmt.Sprintf("%s.%s.svc.cluster.local", inst.Name, inst.Namespace),
	}
}

// allowedHosts builds PAPERLESS_ALLOWED_HOSTS. Omitting the Service name here makes
// Django reject every in-cluster request with a 400, since that name is the Host
// header such a request arrives with; spec.url's own host is added when set.
//
// localhost and 127.0.0.1 are always included: this slice ships no Ingress or
// HTTPRoute, so kubectl port-forward is the only way to reach an instance at all,
// and a browser or client against that tunnel sends one of those two as Host.
func allowedHosts(inst *v1alpha1.PaperlessInstance) string {
	hosts := append(serviceDNSNames(inst), "localhost", "127.0.0.1")
	if inst.Spec.URL != "" {
		if u, err := url.Parse(inst.Spec.URL); err == nil && u.Hostname() != "" {
			hosts = append(hosts, u.Hostname())
		}
	}
	return strings.Join(hosts, ",")
}

// secretKeyEnv sources PAPERLESS_SECRET_KEY from the user's own secret when
// spec.secretKeySecretRef is set, or from the operator-generated one otherwise.
func secretKeyEnv(inst *v1alpha1.PaperlessInstance) corev1.EnvVar {
	if ref := inst.Spec.SecretKeySecretRef; ref != nil {
		return secretEnvVar("PAPERLESS_SECRET_KEY", ref.Name, ref.Key)
	}
	return secretEnvVar("PAPERLESS_SECRET_KEY", SecretKeyName(inst), "PAPERLESS_SECRET_KEY")
}

// cacheEnv sources PAPERLESS_REDIS from the managed Valkey Service, or from the
// external cache otherwise; URLSecretRef wins over a plain URL when both are set,
// matching CacheSpec's own doc comment on which one is preferred.
func cacheEnv(inst *v1alpha1.PaperlessInstance) corev1.EnvVar {
	cache := inst.Spec.Cache
	if cache.IsManaged() {
		return corev1.EnvVar{Name: "PAPERLESS_REDIS", Value: fmt.Sprintf("redis://%s:%d", ValkeyName(inst), valkeyPort)}
	}
	if cache.URLSecretRef != nil {
		return secretEnvVar("PAPERLESS_REDIS", cache.URLSecretRef.Name, cache.URLSecretRef.Key)
	}
	return corev1.EnvVar{Name: "PAPERLESS_REDIS", Value: cache.URL}
}

// adminEnv returns the local superuser's env vars, or none when the admin account is
// disabled: Paperless's bootstrap script needs all three to create one.
func adminEnv(inst *v1alpha1.PaperlessInstance) []corev1.EnvVar {
	admin := inst.Spec.Admin
	if !admin.IsEnabled() {
		return nil
	}

	username := admin.Username
	if username == "" {
		username = defaultAdminUsername
	}

	env := []corev1.EnvVar{{Name: "PAPERLESS_ADMIN_USER", Value: username}}
	if admin.Email != "" {
		env = append(env, corev1.EnvVar{Name: "PAPERLESS_ADMIN_MAIL", Value: admin.Email})
	}

	if ref := admin.PasswordSecretRef; ref != nil {
		env = append(env, secretEnvVar("PAPERLESS_ADMIN_PASSWORD", ref.Name, ref.Key))
	} else {
		env = append(env, secretEnvVar("PAPERLESS_ADMIN_PASSWORD", AdminSecretName(inst), "password"))
	}
	return env
}

// dedupeEnv keeps the last entry for each name, at the position of its first
// appearance. Server-Side Apply treats env as a map keyed by name and rejects a
// duplicate key outright, so an override must replace the earlier entry, not join it.
func dedupeEnv(env []corev1.EnvVar) []corev1.EnvVar {
	index := make(map[string]int, len(env))
	out := make([]corev1.EnvVar, 0, len(env))
	for _, e := range env {
		if i, ok := index[e.Name]; ok {
			out[i] = e
			continue
		}
		index[e.Name] = len(out)
		out = append(out, e)
	}
	return out
}

// workloadEnv assembles the Paperless container's environment. Operator-computed
// values and DatabaseEnv come first, spec.Env last, so dedupeEnv resolves any
// name a user repeats to their value rather than the operator's.
func workloadEnv(inst *v1alpha1.PaperlessInstance) []corev1.EnvVar {
	tz := inst.Spec.Timezone
	if tz == "" {
		tz = defaultTimezone
	}

	ocrLang := strings.Join(inst.Spec.OCR.Languages, "+")
	if ocrLang == "" {
		ocrLang = defaultOCRLanguage
	}

	env := []corev1.EnvVar{
		{Name: "PAPERLESS_ALLOWED_HOSTS", Value: allowedHosts(inst)},
		{Name: "PAPERLESS_TIME_ZONE", Value: tz},
		{Name: "PAPERLESS_OCR_LANGUAGE", Value: ocrLang},
	}

	if inst.Spec.URL != "" {
		env = append(env,
			corev1.EnvVar{Name: "PAPERLESS_URL", Value: inst.Spec.URL},
			corev1.EnvVar{Name: "PAPERLESS_CSRF_TRUSTED_ORIGINS", Value: inst.Spec.URL},
		)
	}

	env = append(env, secretKeyEnv(inst), cacheEnv(inst))
	env = append(env, adminEnv(inst)...)
	env = append(env, DatabaseEnv(inst)...)
	env = append(env, inst.Spec.Env...)

	return dedupeEnv(env)
}

// paperlessVolumeSpecs pairs each volume's name with the PVC it binds to and the path
// Paperless expects it at, matching the upstream project's own docker-compose files.
func paperlessVolumeSpecs(inst *v1alpha1.PaperlessInstance) []struct {
	name, claim, mountPath string
} {
	return []struct{ name, claim, mountPath string }{
		{"data", DataPVCName(inst), "/usr/src/paperless/data"},
		{"media", MediaPVCName(inst), "/usr/src/paperless/media"},
		{"consume", ConsumePVCName(inst), "/usr/src/paperless/consume"},
		{"export", ExportPVCName(inst), "/usr/src/paperless/export"},
	}
}

func paperlessVolumes(inst *v1alpha1.PaperlessInstance) []corev1.Volume {
	specs := paperlessVolumeSpecs(inst)
	volumes := make([]corev1.Volume, 0, len(specs))
	for _, s := range specs {
		volumes = append(volumes, corev1.Volume{
			Name: s.name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: s.claim},
			},
		})
	}
	return volumes
}

func paperlessVolumeMounts(inst *v1alpha1.PaperlessInstance) []corev1.VolumeMount {
	specs := paperlessVolumeSpecs(inst)
	mounts := make([]corev1.VolumeMount, 0, len(specs))
	for _, s := range specs {
		mounts = append(mounts, corev1.VolumeMount{Name: s.name, MountPath: s.mountPath})
	}
	return mounts
}

// Deployment returns the Paperless-NGX Deployment: one replica running the configured
// image with the assembled environment and the four volumes mounted.
func Deployment(inst *v1alpha1.PaperlessInstance) *appsv1.Deployment {
	labels := workloadLabels(inst)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      inst.Name,
			Namespace: inst.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &paperlessReplicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			// Recreate, not the Deployment default of RollingUpdate: the volumes
			// default to ReadWriteOnce, so a second pod cannot start while the first
			// still holds them and a rolling update would deadlock forever.
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ImagePullSecrets: inst.Spec.Image.PullSecrets,
					Containers: []corev1.Container{
						{
							Name:            "paperless",
							Image:           inst.Spec.Image.Reference(),
							ImagePullPolicy: inst.Spec.Image.PullPolicy,
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: paperlessPort},
							},
							Env:          workloadEnv(inst),
							EnvFrom:      inst.Spec.EnvFrom,
							Resources:    inst.Spec.Resources,
							VolumeMounts: paperlessVolumeMounts(inst),
							ReadinessProbe: paperlessProbe(
								inst.Name, readinessInitialDelaySeconds, readinessPeriodSeconds,
								readinessTimeoutSeconds, readinessFailureThreshold,
							),
							LivenessProbe: paperlessProbe(
								inst.Name, livenessInitialDelaySeconds, livenessPeriodSeconds,
								livenessTimeoutSeconds, livenessFailureThreshold,
							),
						},
					},
					Volumes: paperlessVolumes(inst),
				},
			},
		},
	}
}

// Service returns the in-cluster Service fronting the Paperless Deployment. Its
// selector must match the Deployment's pod labels exactly, or it routes to nothing.
func Service(inst *v1alpha1.PaperlessInstance) *corev1.Service {
	labels := workloadLabels(inst)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      inst.Name,
			Namespace: inst.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{Name: "http", Port: paperlessPort, TargetPort: intstr.FromInt32(paperlessPort)},
			},
		},
	}
}
