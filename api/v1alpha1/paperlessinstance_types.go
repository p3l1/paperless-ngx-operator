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

	// Digest pins an exact image, in the form "sha256:<64 hex characters>", and
	// takes precedence over Tag.
	// +kubebuilder:validation:Pattern="^sha256:[0-9a-f]{64}$"
	// +optional
	Digest string `json:"digest,omitempty"`

	// PullPolicy controls when the kubelet re-pulls the image; the default only
	// pulls when it is not already present on the node.
	// +kubebuilder:default="IfNotPresent"
	// +optional
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`

	// PullSecrets authenticate image pulls from a private registry.
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

// Default volume sizes. Each must match its field's "+kubebuilder:default={size:"…"}"
// marker below; TestVolumeSizeDefaultsMatchMarkers fails if the two drift apart, since
// a kubebuilder marker cannot reference a Go identifier and so cannot enforce it itself.
const (
	DefaultDataSize     = "5Gi"
	DefaultMediaSize    = "20Gi"
	DefaultConsumeSize  = "5Gi"
	DefaultExportSize   = "10Gi"
	DefaultDatabaseSize = "10Gi"
)

// VolumeSpec describes one persistent volume claim.
type VolumeSpec struct {
	// Size is the storage capacity requested for the claim.
	// +optional
	Size resource.Quantity `json:"size,omitempty"`

	// StorageClassName is omitted from the claim when unset, letting the cluster
	// choose its default. An empty string disables dynamic provisioning.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`

	// AccessModes the claim requests; the storage class's default applies when unset.
	// +optional
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
}

// SizeOrDefault returns Size, or fallback parsed as a resource.Quantity when Size is
// the zero quantity. A typed Go client always sends Size as zero rather than omitting
// it (encoding/json never omits a zero-value struct), and a zero-size PVC is never a
// legitimate request — the API server rejects it outright — so resolving the fallback
// here, rather than at each call site, is the one place every builder should read a
// volume's size through instead of touching Size directly.
func (v VolumeSpec) SizeOrDefault(fallback string) resource.Quantity {
	if v.Size.IsZero() {
		return resource.MustParse(fallback)
	}
	return v.Size
}

// StorageSpec groups the four volumes Paperless uses. Sizes are set per volume,
// rather than sharing VolumeSpec's own default, because they legitimately differ:
// media holds every document and needs far more room than the others.
type StorageSpec struct {
	// Data holds Paperless's own working state: the search index, the
	// classification model, and SQLite artefacts if used. Needs a
	// ReadWriteMany-capable storage class if the Deployment ever runs more than
	// one replica.
	// +kubebuilder:default={size:"5Gi"}
	// +optional
	Data VolumeSpec `json:"data,omitempty"`

	// Media holds the document archive itself: originals, archived PDFs, and
	// thumbnails. It grows without bound and is the volume to size deliberately,
	// since not every storage class supports expanding a claim later. Needs a
	// ReadWriteMany-capable storage class if the Deployment ever runs more than
	// one replica.
	// +kubebuilder:default={size:"20Gi"}
	// +optional
	Media VolumeSpec `json:"media,omitempty"`

	// Consume is the watched inbox: documents placed here are picked up for
	// ingestion and removed once processed. Needs a ReadWriteMany-capable storage
	// class if the Deployment ever runs more than one replica.
	// +kubebuilder:default={size:"5Gi"}
	// +optional
	Consume VolumeSpec `json:"consume,omitempty"`

	// Export is the destination for document_exporter runs, and doubles as where a
	// backup lands. Needs a ReadWriteMany-capable storage class if the Deployment
	// ever runs more than one replica.
	// +kubebuilder:default={size:"10Gi"}
	// +optional
	Export VolumeSpec `json:"export,omitempty"`
}

// ExternalDatabase points at a PostgreSQL the operator does not manage.
type ExternalDatabase struct {
	// Host is the PostgreSQL server's DNS name or IP address.
	// +kubebuilder:validation:Required
	Host string `json:"host"`

	// Port the PostgreSQL server listens on.
	// +kubebuilder:default=5432
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// Name of the database to connect to inside the PostgreSQL server. This is a
	// database name, not a Kubernetes object name.
	// +kubebuilder:default="paperless"
	// +optional
	Name string `json:"name,omitempty"`

	// CredentialsSecretRef must hold "username" and "password" keys.
	// +kubebuilder:validation:Required
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`
}

// ManagedDatabase describes a CloudNativePG cluster the operator creates.
type ManagedDatabase struct {
	// Instances is the number of PostgreSQL instances CloudNativePG runs for
	// high availability.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Instances int32 `json:"instances,omitempty"`

	// Storage is the PersistentVolumeClaim template CloudNativePG uses for each instance.
	// +kubebuilder:default={size:"10Gi"}
	// +optional
	Storage VolumeSpec `json:"storage,omitempty"`
}

// DatabaseSpec selects between a managed and an external database. Setting both
// CNPG and External is rejected: the operator has no way to tell which one the
// instance should actually use.
// +kubebuilder:validation:XValidation:rule="!(has(self.cnpg) && has(self.external))",message="set either database.cnpg or database.external, not both"
type DatabaseSpec struct {
	// CNPG configures the CloudNativePG cluster the operator creates and manages.
	// +optional
	CNPG *ManagedDatabase `json:"cnpg,omitempty"`

	// External points at a PostgreSQL database the operator does not manage.
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
	// Managed selects whether the operator runs its own Valkey instance. Set this
	// to false and supply URL or URLSecretRef to use an external Redis-compatible cache.
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

// IsManaged reports whether the operator runs its own cache; nil (unset) defaults to true.
func (c CacheSpec) IsManaged() bool { return c.Managed == nil || *c.Managed }

// AdminSpec controls the local superuser Paperless creates at startup. Set Enabled
// to false for an installation where authentication is delegated entirely to an
// OIDC provider and no local superuser account should exist.
type AdminSpec struct {
	// Enabled controls whether the operator ensures a local superuser account exists.
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Username of the local superuser account.
	// +kubebuilder:default="admin"
	// +optional
	Username string `json:"username,omitempty"`

	// Email address recorded on the superuser account.
	// +optional
	Email string `json:"email,omitempty"`

	// PasswordSecretRef supplies the password; one is generated when unset.
	// +optional
	PasswordSecretRef *corev1.SecretKeySelector `json:"passwordSecretRef,omitempty"`
}

// IsEnabled reports whether the operator should ensure a local superuser exists;
// nil (unset) defaults to true.
func (a AdminSpec) IsEnabled() bool { return a.Enabled == nil || *a.Enabled }

// OCRSpec configures text recognition.
type OCRSpec struct {
	// Languages are Tesseract language codes (e.g. "eng", "deu") to recognise text
	// in. Each one must already be installed in the image; an unavailable code
	// degrades OCR silently rather than failing.
	// +kubebuilder:default={"eng"}
	// +optional
	Languages []string `json:"languages,omitempty"`
}

// PaperlessInstanceSpec describes one Paperless-NGX installation.
type PaperlessInstanceSpec struct {
	// Image selects the Paperless-NGX container image.
	// +kubebuilder:default={}
	// +optional
	Image ImageSpec `json:"image,omitempty"`

	// URL is the external address Paperless builds links and callbacks from, e.g.
	// https://paperless.example.org.
	// +kubebuilder:validation:Pattern="^https?://"
	// +optional
	URL string `json:"url,omitempty"`

	// Timezone is an IANA tz database name, e.g. "Europe/Berlin", that Paperless
	// uses to display and file dates.
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

	// Database selects between a managed CloudNativePG cluster and an external
	// PostgreSQL server.
	// +optional
	Database DatabaseSpec `json:"database,omitempty"`

	// Cache selects between a managed Valkey instance and an external
	// Redis-compatible cache.
	// +kubebuilder:default={}
	// +optional
	Cache CacheSpec `json:"cache,omitempty"`

	// Storage configures the four PersistentVolumeClaims Paperless uses.
	// +kubebuilder:default={}
	// +optional
	Storage StorageSpec `json:"storage,omitempty"`

	// Resources applied to the Paperless container.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Env passes any PAPERLESS_* variable the typed fields do not cover.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// EnvFrom loads environment variables in bulk from ConfigMaps or Secrets. Env
	// entries take precedence over an EnvFrom entry of the same name.
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

	// ObservedGeneration is the metadata.generation the conditions above were
	// last computed from.
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

// GetConditions returns a pointer to the status conditions slice, letting a
// shared condition-setting helper mutate it in place. A later CRD in this
// operator can implement the same method to reuse that helper unchanged.
func (in *PaperlessInstance) GetConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

// +kubebuilder:object:root=true

// PaperlessInstanceList contains a list of PaperlessInstance.
type PaperlessInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the list of PaperlessInstance resources returned by this request.
	Items []PaperlessInstance `json:"items"`
}
