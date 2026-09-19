// SPDX-License-Identifier: AGPL-3.0-only

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition types reported in status.conditions.
const (
	// ConditionReady summarises whether the instance serves requests.
	ConditionReady = "Ready"
	// ConditionDatabaseReady tracks the database the instance depends on.
	ConditionDatabaseReady = "DatabaseReady"
)

// defaultRepository is the image used when spec.image.repository is unset. Kept in
// sync by hand with the kubebuilder marker below: markers cannot reference Go identifiers.
const defaultRepository = "ghcr.io/paperless-ngx/paperless-ngx"

// ImageSpec selects the Paperless-NGX container image.
type ImageSpec struct {
	// Repository holds the image without tag or digest.
	// +kubebuilder:default="ghcr.io/paperless-ngx/paperless-ngx"
	// +optional
	Repository string `json:"repository,omitempty"`

	// Tag selects a released version. Ignored when Digest is set.
	// +kubebuilder:default="3.1.3"
	// +optional
	Tag string `json:"tag,omitempty"`

	// Digest pins an exact image and takes precedence over Tag.
	// +optional
	Digest string `json:"digest,omitempty"`

	// +kubebuilder:default="IfNotPresent"
	// +optional
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`

	// +optional
	PullSecrets []corev1.LocalObjectReference `json:"pullSecrets,omitempty"`
}

// Reference renders the image reference the pod should run. It never invents a tag:
// an empty Tag with no Digest set yields a reference with an empty tag suffix.
func (i ImageSpec) Reference() string {
	repo := i.Repository
	if repo == "" {
		repo = defaultRepository
	}
	if i.Digest != "" {
		return repo + "@" + i.Digest
	}
	return repo + ":" + i.Tag
}

// VolumeSpec describes one persistent volume claim.
type VolumeSpec struct {
	// +optional
	Size resource.Quantity `json:"size,omitempty"`

	// StorageClassName is omitted from the claim when unset, letting the cluster
	// choose its default. An empty string disables dynamic provisioning.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`

	// +optional
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
}

// StorageSpec groups the four volumes Paperless uses. Sizes are set per volume,
// rather than sharing VolumeSpec's own default, because they legitimately differ:
// media holds every document and needs far more room than the others.
type StorageSpec struct {
	// +kubebuilder:default={size:"5Gi"}
	// +optional
	Data VolumeSpec `json:"data,omitempty"`
	// +kubebuilder:default={size:"20Gi"}
	// +optional
	Media VolumeSpec `json:"media,omitempty"`
	// +kubebuilder:default={size:"5Gi"}
	// +optional
	Consume VolumeSpec `json:"consume,omitempty"`
	// +kubebuilder:default={size:"10Gi"}
	// +optional
	Export VolumeSpec `json:"export,omitempty"`
}

// ExternalDatabase points at a PostgreSQL the operator does not manage.
type ExternalDatabase struct {
	// +kubebuilder:validation:Required
	Host string `json:"host"`

	// +kubebuilder:default=5432
	// +optional
	Port int32 `json:"port,omitempty"`

	// +kubebuilder:default="paperless"
	// +optional
	Name string `json:"name,omitempty"`

	// CredentialsSecretRef must hold "username" and "password" keys.
	// +kubebuilder:validation:Required
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`
}

// ManagedDatabase describes a CloudNativePG cluster the operator creates.
type ManagedDatabase struct {
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Instances int32 `json:"instances,omitempty"`

	// +kubebuilder:default={size:"10Gi"}
	// +optional
	Storage VolumeSpec `json:"storage,omitempty"`
}

// DatabaseSpec selects between a managed and an external database. Setting both
// CNPG and External is rejected: the operator has no way to tell which one the
// instance should actually use.
// +kubebuilder:validation:XValidation:rule="!(has(self.cnpg) && has(self.external))",message="set either database.cnpg or database.external, not both"
type DatabaseSpec struct {
	// +optional
	CNPG *ManagedDatabase `json:"cnpg,omitempty"`

	// +optional
	External *ExternalDatabase `json:"external,omitempty"`
}

// IsExternal reports whether the instance uses a database the operator does not manage.
func (d DatabaseSpec) IsExternal() bool { return d.External != nil }

// CacheSpec selects between a managed Valkey and an external Redis-compatible cache.
// Setting URL or URLSecretRef while Managed is true (or unset) is rejected, since the
// operator would silently ignore them; setting neither while Managed is false is
// rejected too, since Paperless cannot start without a cache.
// +kubebuilder:validation:XValidation:rule="(!has(self.managed) || self.managed) ? (!has(self.url) && !has(self.urlSecretRef)) : ((has(self.url) && self.url != \"\") || has(self.urlSecretRef))",message="set cache.url or cache.urlSecretRef when cache.managed is false; leave both unset when the operator manages the cache"
type CacheSpec struct {
	// +kubebuilder:default=true
	// +optional
	Managed *bool `json:"managed,omitempty"`

	// URL is used when Managed is false and URLSecretRef is unset, e.g.
	// redis://cache:6379. Prefer URLSecretRef when the URL carries a password:
	// this field is stored as plain text.
	// +optional
	URL string `json:"url,omitempty"`

	// URLSecretRef supplies the cache URL from a Secret key, used when Managed is
	// false. Preferred over URL whenever the connection string carries a credential.
	// +optional
	URLSecretRef *corev1.SecretKeySelector `json:"urlSecretRef,omitempty"`
}

// AdminSpec controls the local superuser Paperless creates at startup.
type AdminSpec struct {
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// +kubebuilder:default="admin"
	// +optional
	Username string `json:"username,omitempty"`

	// +optional
	Email string `json:"email,omitempty"`

	// PasswordSecretRef supplies the password; one is generated when unset.
	// +optional
	PasswordSecretRef *corev1.SecretKeySelector `json:"passwordSecretRef,omitempty"`
}

// OCRSpec configures text recognition.
type OCRSpec struct {
	// +kubebuilder:default={"eng"}
	// +optional
	Languages []string `json:"languages,omitempty"`
}

// PaperlessInstanceSpec describes one Paperless-NGX installation.
type PaperlessInstanceSpec struct {
	// +kubebuilder:default={}
	// +optional
	Image ImageSpec `json:"image,omitempty"`

	// URL is the external address Paperless builds links and callbacks from.
	// +optional
	URL string `json:"url,omitempty"`

	// +kubebuilder:default="UTC"
	// +optional
	Timezone string `json:"timezone,omitempty"`

	// +kubebuilder:default={}
	// +optional
	OCR OCRSpec `json:"ocr,omitempty"`

	// +kubebuilder:default={}
	// +optional
	Admin AdminSpec `json:"admin,omitempty"`

	// SecretKeySecretRef supplies Django's secret key; one is generated when unset.
	// +optional
	SecretKeySecretRef *corev1.SecretKeySelector `json:"secretKeySecretRef,omitempty"`

	// +optional
	Database DatabaseSpec `json:"database,omitempty"`

	// +kubebuilder:default={}
	// +optional
	Cache CacheSpec `json:"cache,omitempty"`

	// +kubebuilder:default={}
	// +optional
	Storage StorageSpec `json:"storage,omitempty"`

	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Env passes any PAPERLESS_* variable the typed fields do not cover.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// +optional
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`
}

// PaperlessInstanceStatus reports what the operator observes.
type PaperlessInstanceStatus struct {
	// Conditions are the operator's most recent observations of the instance's
	// state, keyed by type so Server-Side Apply merges entries instead of
	// replacing the whole list.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// URL echoes the address the instance answers on.
	// +optional
	URL string `json:"url,omitempty"`

	// Version is what the running instance reports about itself.
	// +optional
	Version string `json:"version,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=pli
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PaperlessInstance is one Paperless-NGX installation.
type PaperlessInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:default={}
	// +optional
	Spec   PaperlessInstanceSpec   `json:"spec,omitempty"`
	Status PaperlessInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PaperlessInstanceList contains a list of PaperlessInstance.
type PaperlessInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PaperlessInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PaperlessInstance{}, &PaperlessInstanceList{})
}
