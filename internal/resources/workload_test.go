// SPDX-License-Identifier: AGPL-3.0-only

package resources

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// mountPath returns the path the named volume is mounted at, failing the test if the
// container has no such mount.
func mountPath(t *testing.T, mounts []corev1.VolumeMount, name string) string {
	t.Helper()
	for _, m := range mounts {
		if m.Name == name {
			return m.MountPath
		}
	}
	t.Fatalf("no volume mount named %q among %d mounts", name, len(mounts))
	return ""
}

// envAbsent fails the test if any entry named name exists.
func envAbsent(t *testing.T, env []corev1.EnvVar, name string) {
	t.Helper()
	for _, e := range env {
		if e.Name == name {
			t.Fatalf("env var %q is present with value %q, want absent", name, e.Value)
		}
	}
}

func TestDeploymentEnvIncludesServiceNameInAllowedHosts(t *testing.T) {
	inst := instance() // spec.url unset

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	hosts := envValue(t, env, "PAPERLESS_ALLOWED_HOSTS")
	if !strings.Contains(hosts, inst.Name) {
		t.Errorf("PAPERLESS_ALLOWED_HOSTS = %q, want it to contain the Service name %q", hosts, inst.Name)
	}

	// Without spec.url there is nothing to build an origin from.
	envAbsent(t, env, "PAPERLESS_URL")
	envAbsent(t, env, "PAPERLESS_CSRF_TRUSTED_ORIGINS")
}

func TestDeploymentEnvURLSetsURLCSRFAndAllowedHosts(t *testing.T) {
	inst := instance()
	inst.Spec.URL = "https://documents.example.org"

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	if got := envValue(t, env, "PAPERLESS_URL"); got != inst.Spec.URL {
		t.Errorf("PAPERLESS_URL = %q, want %q", got, inst.Spec.URL)
	}
	if got := envValue(t, env, "PAPERLESS_CSRF_TRUSTED_ORIGINS"); got != inst.Spec.URL {
		t.Errorf("PAPERLESS_CSRF_TRUSTED_ORIGINS = %q, want %q", got, inst.Spec.URL)
	}

	hosts := envValue(t, env, "PAPERLESS_ALLOWED_HOSTS")
	if !strings.Contains(hosts, inst.Name) {
		t.Errorf("PAPERLESS_ALLOWED_HOSTS = %q, want it to contain the Service name %q", hosts, inst.Name)
	}
	if !strings.Contains(hosts, "documents.example.org") {
		t.Errorf("PAPERLESS_ALLOWED_HOSTS = %q, want it to contain the external host from spec.url", hosts)
	}
}

func TestDeploymentEnvSetsPostgresEngine(t *testing.T) {
	inst := instance()

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	if got := envValue(t, env, "PAPERLESS_DBENGINE"); got != "postgresql" {
		t.Errorf("PAPERLESS_DBENGINE = %q, want %q", got, "postgresql")
	}
}

func TestDeploymentEnvDefaultsTimeZoneAndOCRLanguage(t *testing.T) {
	inst := instance() // spec.timezone and spec.ocr.languages both unset

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	if got := envValue(t, env, "PAPERLESS_TIME_ZONE"); got != "UTC" {
		t.Errorf("PAPERLESS_TIME_ZONE = %q, want %q", got, "UTC")
	}
	if got := envValue(t, env, "PAPERLESS_OCR_LANGUAGE"); got != "eng" {
		t.Errorf("PAPERLESS_OCR_LANGUAGE = %q, want %q", got, "eng")
	}
}

func TestDeploymentEnvTimeZoneAndOCRLanguageFromSpec(t *testing.T) {
	inst := instance()
	inst.Spec.Timezone = "Europe/Berlin"
	inst.Spec.OCR.Languages = []string{"deu", "eng"}

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	if got := envValue(t, env, "PAPERLESS_TIME_ZONE"); got != "Europe/Berlin" {
		t.Errorf("PAPERLESS_TIME_ZONE = %q, want %q", got, "Europe/Berlin")
	}
	// Tesseract combines multiple languages with "+", not a comma.
	if got := envValue(t, env, "PAPERLESS_OCR_LANGUAGE"); got != "deu+eng" {
		t.Errorf("PAPERLESS_OCR_LANGUAGE = %q, want %q", got, "deu+eng")
	}
}

// The escape hatch in spec.env is fake unless it actually wins; this is the test the
// whole task hinges on.
func TestDeploymentEnvUserOverrideWinsOverOperatorDefault(t *testing.T) {
	inst := instance()
	inst.Spec.Env = []corev1.EnvVar{
		{Name: "PAPERLESS_TIME_ZONE", Value: "Europe/Berlin"},
	}

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	if got := envValue(t, env, "PAPERLESS_TIME_ZONE"); got != "Europe/Berlin" {
		t.Errorf("PAPERLESS_TIME_ZONE = %q, want the user override %q", got, "Europe/Berlin")
	}
}

// A user overriding a variable DatabaseEnv itself sets must still win: the override
// applies uniformly, not just to variables this builder computes directly.
func TestDeploymentEnvUserOverrideWinsOverDatabaseEnv(t *testing.T) {
	inst := instance()
	inst.Spec.Env = []corev1.EnvVar{
		{Name: "PAPERLESS_DBENGINE", Value: "sqlite"},
	}

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	if got := envValue(t, env, "PAPERLESS_DBENGINE"); got != "sqlite" {
		t.Errorf("PAPERLESS_DBENGINE = %q, want the user override %q", got, "sqlite")
	}
}

func TestDeploymentEnvUnknownUserVariablePassesThrough(t *testing.T) {
	inst := instance()
	inst.Spec.Env = []corev1.EnvVar{
		{Name: "PAPERLESS_TASK_WORKERS", Value: "4"},
	}

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	if got := envValue(t, env, "PAPERLESS_TASK_WORKERS"); got != "4" {
		t.Errorf("PAPERLESS_TASK_WORKERS = %q, want %q", got, "4")
	}
}

// Server-Side Apply treats env as a map keyed by name and rejects a duplicate key
// outright, so an override must replace the operator's entry, never sit beside it.
func TestDeploymentEnvHasNoDuplicateNames(t *testing.T) {
	inst := instance()
	inst.Spec.Env = []corev1.EnvVar{
		{Name: "PAPERLESS_TIME_ZONE", Value: "Europe/Berlin"},
		{Name: "PAPERLESS_DBENGINE", Value: "sqlite"},
	}

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	seen := map[string]int{}
	for _, e := range env {
		seen[e.Name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("env var %q appears %d times, want at most once", name, count)
		}
	}
}

func TestDeploymentEnvFromPassesThroughUserSpec(t *testing.T) {
	inst := instance()
	inst.Spec.EnvFrom = []corev1.EnvFromSource{
		{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "extra-config"}}},
	}

	c := Deployment(inst).Spec.Template.Spec.Containers[0]

	if len(c.EnvFrom) != 1 || c.EnvFrom[0].ConfigMapRef.Name != "extra-config" {
		t.Errorf("EnvFrom = %+v, want a single ref to %q", c.EnvFrom, "extra-config")
	}
}

func TestDeploymentSecretKeyEnvSourcesGeneratedSecretByDefault(t *testing.T) {
	inst := instance()

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	secretName, key := envSecretRef(t, env, "PAPERLESS_SECRET_KEY")
	if secretName != SecretKeyName(inst) || key != "PAPERLESS_SECRET_KEY" {
		t.Errorf("PAPERLESS_SECRET_KEY = secret %q key %q, want secret %q key %q", secretName, key, SecretKeyName(inst), "PAPERLESS_SECRET_KEY")
	}
}

func TestDeploymentSecretKeyEnvSourcesUserSecretWhenRefSet(t *testing.T) {
	inst := instance()
	inst.Spec.SecretKeySecretRef = &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
		Key:                  "key",
	}

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	secretName, key := envSecretRef(t, env, "PAPERLESS_SECRET_KEY")
	if secretName != "my-secret" || key != "key" {
		t.Errorf("PAPERLESS_SECRET_KEY = secret %q key %q, want secret %q key %q", secretName, key, "my-secret", "key")
	}
}

func TestDeploymentAdminEnvSetWhenEnabled(t *testing.T) {
	inst := instance() // spec.admin is unset, which IsEnabled() treats as true

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	if got := envValue(t, env, "PAPERLESS_ADMIN_USER"); got != "admin" {
		t.Errorf("PAPERLESS_ADMIN_USER = %q, want %q", got, "admin")
	}
	secretName, key := envSecretRef(t, env, "PAPERLESS_ADMIN_PASSWORD")
	if secretName != AdminSecretName(inst) || key != "password" {
		t.Errorf("PAPERLESS_ADMIN_PASSWORD = secret %q key %q, want secret %q key %q", secretName, key, AdminSecretName(inst), "password")
	}
	envAbsent(t, env, "PAPERLESS_ADMIN_MAIL") // spec.admin.email is unset
}

func TestDeploymentAdminEnvOmittedWhenDisabled(t *testing.T) {
	inst := instance()
	disabled := false
	inst.Spec.Admin.Enabled = &disabled

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	envAbsent(t, env, "PAPERLESS_ADMIN_USER")
	envAbsent(t, env, "PAPERLESS_ADMIN_PASSWORD")
	envAbsent(t, env, "PAPERLESS_ADMIN_MAIL")
}

func TestDeploymentAdminEnvUsesSpecUsernameAndEmail(t *testing.T) {
	inst := instance()
	inst.Spec.Admin.Username = "operator"
	inst.Spec.Admin.Email = "operator@example.org"

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	if got := envValue(t, env, "PAPERLESS_ADMIN_USER"); got != "operator" {
		t.Errorf("PAPERLESS_ADMIN_USER = %q, want %q", got, "operator")
	}
	if got := envValue(t, env, "PAPERLESS_ADMIN_MAIL"); got != "operator@example.org" {
		t.Errorf("PAPERLESS_ADMIN_MAIL = %q, want %q", got, "operator@example.org")
	}
}

func TestDeploymentAdminEnvSourcesUserPasswordSecretWhenRefSet(t *testing.T) {
	inst := instance()
	inst.Spec.Admin.PasswordSecretRef = &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "my-admin"},
		Key:                  "pw",
	}

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	secretName, key := envSecretRef(t, env, "PAPERLESS_ADMIN_PASSWORD")
	if secretName != "my-admin" || key != "pw" {
		t.Errorf("PAPERLESS_ADMIN_PASSWORD = secret %q key %q, want secret %q key %q", secretName, key, "my-admin", "pw")
	}
}

func TestDeploymentCacheEnvPointsAtManagedValkeyByDefault(t *testing.T) {
	inst := instance() // spec.cache.managed is unset, which IsManaged() treats as true

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	want := "redis://" + ValkeyName(inst) + ":6379"
	if got := envValue(t, env, "PAPERLESS_REDIS"); got != want {
		t.Errorf("PAPERLESS_REDIS = %q, want %q", got, want)
	}
}

func TestDeploymentCacheEnvUsesExternalURLWhenUnmanaged(t *testing.T) {
	inst := instance()
	managed := false
	inst.Spec.Cache.Managed = &managed
	inst.Spec.Cache.URL = "redis://cache.example.org:6379"

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	if got := envValue(t, env, "PAPERLESS_REDIS"); got != inst.Spec.Cache.URL {
		t.Errorf("PAPERLESS_REDIS = %q, want %q", got, inst.Spec.Cache.URL)
	}
}

func TestDeploymentCacheEnvUsesURLSecretRefWhenUnmanaged(t *testing.T) {
	inst := instance()
	managed := false
	inst.Spec.Cache.Managed = &managed
	inst.Spec.Cache.URLSecretRef = &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "cache-creds"},
		Key:                  "url",
	}

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	secretName, key := envSecretRef(t, env, "PAPERLESS_REDIS")
	if secretName != "cache-creds" || key != "url" {
		t.Errorf("PAPERLESS_REDIS = secret %q key %q, want secret %q key %q", secretName, key, "cache-creds", "url")
	}
}

// CacheSpec's own doc comment says URLSecretRef is preferred over URL whenever both
// carry a value; nothing prevents a raw struct bypassing the CEL rule from setting both.
func TestDeploymentCacheEnvPrefersURLSecretRefWhenBothSet(t *testing.T) {
	inst := instance()
	managed := false
	inst.Spec.Cache.Managed = &managed
	inst.Spec.Cache.URL = "redis://cache.example.org:6379"
	inst.Spec.Cache.URLSecretRef = &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "cache-creds"},
		Key:                  "url",
	}

	env := Deployment(inst).Spec.Template.Spec.Containers[0].Env

	secretName, key := envSecretRef(t, env, "PAPERLESS_REDIS")
	if secretName != "cache-creds" || key != "url" {
		t.Errorf("PAPERLESS_REDIS = secret %q key %q, want the secretRef %q key %q", secretName, key, "cache-creds", "url")
	}
}

func TestDeploymentVolumeMountsAtExpectedPaths(t *testing.T) {
	inst := instance()

	mounts := Deployment(inst).Spec.Template.Spec.Containers[0].VolumeMounts

	cases := []struct {
		name string
		want string
	}{
		{"data", "/usr/src/paperless/data"},
		{"media", "/usr/src/paperless/media"},
		{"consume", "/usr/src/paperless/consume"},
		{"export", "/usr/src/paperless/export"},
	}
	for _, c := range cases {
		if got := mountPath(t, mounts, c.name); got != c.want {
			t.Errorf("mount %q path = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDeploymentVolumesReferencePVCsByName(t *testing.T) {
	inst := instance()

	volumes := Deployment(inst).Spec.Template.Spec.Volumes

	cases := []struct {
		name string
		want string
	}{
		{"data", DataPVCName(inst)},
		{"media", MediaPVCName(inst)},
		{"consume", ConsumePVCName(inst)},
		{"export", ExportPVCName(inst)},
	}
	for _, c := range cases {
		found := false
		for _, v := range volumes {
			if v.Name == c.name {
				found = true
				if v.PersistentVolumeClaim == nil || v.PersistentVolumeClaim.ClaimName != c.want {
					t.Errorf("volume %q claim = %+v, want claim %q", c.name, v.PersistentVolumeClaim, c.want)
				}
			}
		}
		if !found {
			t.Errorf("no volume named %q", c.name)
		}
	}
}

func TestDeploymentImageUsesDigestWhenSet(t *testing.T) {
	inst := instance()
	digest := "sha256:" + strings.Repeat("a", 64)
	inst.Spec.Image.Repository = "example.org/paperless"
	inst.Spec.Image.Tag = "3.1.3"
	inst.Spec.Image.Digest = digest

	got := Deployment(inst).Spec.Template.Spec.Containers[0].Image
	want := "example.org/paperless@" + digest
	if got != want {
		t.Errorf("image = %q, want %q (digest must win over tag)", got, want)
	}
}

func TestDeploymentImagePullPolicyAndPullSecretsFromSpec(t *testing.T) {
	inst := instance()
	inst.Spec.Image.PullPolicy = corev1.PullAlways
	inst.Spec.Image.PullSecrets = []corev1.LocalObjectReference{{Name: "registry-creds"}}

	d := Deployment(inst)
	c := d.Spec.Template.Spec.Containers[0]

	if c.ImagePullPolicy != corev1.PullAlways {
		t.Errorf("ImagePullPolicy = %q, want %q", c.ImagePullPolicy, corev1.PullAlways)
	}
	if len(d.Spec.Template.Spec.ImagePullSecrets) != 1 || d.Spec.Template.Spec.ImagePullSecrets[0].Name != "registry-creds" {
		t.Errorf("ImagePullSecrets = %+v, want a single ref to %q", d.Spec.Template.Spec.ImagePullSecrets, "registry-creds")
	}
}

func TestDeploymentNameAndNamespace(t *testing.T) {
	inst := instance()

	d := Deployment(inst)
	if got, want := d.Name, inst.Name; got != want {
		t.Errorf("name = %q, want %q", got, want)
	}
	if got, want := d.Namespace, inst.Namespace; got != want {
		t.Errorf("namespace = %q, want %q", got, want)
	}
}

// A RollingUpdate default would try to start a second pod before the first releases the
// four ReadWriteOnce volumes, deadlocking every rollout.
func TestDeploymentUsesRecreateStrategy(t *testing.T) {
	inst := instance()

	if got, want := Deployment(inst).Spec.Strategy.Type, appsv1.RecreateDeploymentStrategyType; got != want {
		t.Errorf("strategy = %q, want %q", got, want)
	}
}

func TestDeploymentSelectorMatchesPodTemplateLabels(t *testing.T) {
	inst := instance()

	d := Deployment(inst)
	if d.Spec.Selector == nil {
		t.Fatal("Selector is nil")
	}
	for k, v := range d.Spec.Selector.MatchLabels {
		if d.Spec.Template.Labels[k] != v {
			t.Errorf("pod template label %q = %q, selector wants %q", k, d.Spec.Template.Labels[k], v)
		}
	}
}

func TestServiceSelectorMatchesDeploymentPodLabels(t *testing.T) {
	inst := instance()

	s := Service(inst)
	d := Deployment(inst)

	if len(s.Spec.Selector) == 0 {
		t.Fatal("Service selector is empty")
	}
	for k, v := range s.Spec.Selector {
		if d.Spec.Template.Labels[k] != v {
			t.Errorf("service selector %q = %q, Deployment pod label is %q", k, v, d.Spec.Template.Labels[k])
		}
	}
}

func TestServiceNameNamespaceAndPort(t *testing.T) {
	inst := instance()

	s := Service(inst)
	if got, want := s.Name, inst.Name; got != want {
		t.Errorf("name = %q, want %q", got, want)
	}
	if got, want := s.Namespace, inst.Namespace; got != want {
		t.Errorf("namespace = %q, want %q", got, want)
	}
	if len(s.Spec.Ports) != 1 || s.Spec.Ports[0].Port != 8000 {
		t.Errorf("service ports = %+v, want a single 8000", s.Spec.Ports)
	}
}

func TestDeploymentAndServiceCarryLabelsAndNamespace(t *testing.T) {
	inst := instance()

	for _, obj := range []struct {
		name      string
		namespace string
		labels    map[string]string
	}{
		{Deployment(inst).Name, Deployment(inst).Namespace, Deployment(inst).Labels},
		{Service(inst).Name, Service(inst).Namespace, Service(inst).Labels},
	} {
		if obj.namespace != inst.Namespace {
			t.Errorf("%s: namespace = %q, want %q", obj.name, obj.namespace, inst.Namespace)
		}
		if obj.labels["app.kubernetes.io/instance"] != inst.Name {
			t.Errorf("%s: app.kubernetes.io/instance = %q, want %q", obj.name, obj.labels["app.kubernetes.io/instance"], inst.Name)
		}
	}
}

// Readiness must be generous: a fresh instance runs database migrations before it
// starts answering, and a probe that gives up too early flaps the pod forever.
func TestDeploymentReadinessProbeIsHTTPGetOnRootAtContainerPort(t *testing.T) {
	inst := instance()

	c := Deployment(inst).Spec.Template.Spec.Containers[0]
	p := c.ReadinessProbe
	if p == nil {
		t.Fatal("ReadinessProbe is nil")
	}
	if p.HTTPGet == nil {
		t.Fatal("ReadinessProbe.HTTPGet is nil")
	}
	if p.HTTPGet.Path != "/" {
		t.Errorf("ReadinessProbe path = %q, want %q", p.HTTPGet.Path, "/")
	}
	if p.HTTPGet.Port.IntValue() != paperlessPort {
		t.Errorf("ReadinessProbe port = %v, want %d", p.HTTPGet.Port, paperlessPort)
	}
}

func TestDeploymentLivenessProbeIsHTTPGetOnRootAtContainerPort(t *testing.T) {
	inst := instance()

	c := Deployment(inst).Spec.Template.Spec.Containers[0]
	p := c.LivenessProbe
	if p == nil {
		t.Fatal("LivenessProbe is nil")
	}
	if p.HTTPGet == nil {
		t.Fatal("LivenessProbe.HTTPGet is nil")
	}
	if p.HTTPGet.Path != "/" {
		t.Errorf("LivenessProbe path = %q, want %q", p.HTTPGet.Path, "/")
	}
	if p.HTTPGet.Port.IntValue() != paperlessPort {
		t.Errorf("LivenessProbe port = %v, want %d", p.HTTPGet.Port, paperlessPort)
	}
}

// Restarting a container mid-migration is worse than a slow-to-appear Service
// endpoint, so liveness must tolerate a longer stretch of failure than readiness.
func TestDeploymentLivenessProbeIsMorePatientThanReadiness(t *testing.T) {
	inst := instance()

	c := Deployment(inst).Spec.Template.Spec.Containers[0]
	readinessWindow := c.ReadinessProbe.InitialDelaySeconds + c.ReadinessProbe.FailureThreshold*c.ReadinessProbe.PeriodSeconds
	livenessWindow := c.LivenessProbe.InitialDelaySeconds + c.LivenessProbe.FailureThreshold*c.LivenessProbe.PeriodSeconds

	if livenessWindow <= readinessWindow {
		t.Errorf("liveness window = %ds, want it greater than the readiness window %ds", livenessWindow, readinessWindow)
	}
}

func TestDeploymentResourcesFromSpec(t *testing.T) {
	inst := instance()
	inst.Spec.Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
	}

	c := Deployment(inst).Spec.Template.Spec.Containers[0]
	if _, ok := c.Resources.Requests[corev1.ResourceCPU]; !ok {
		t.Error("container resources do not carry the CPU request from spec.resources")
	}
}
