// SPDX-License-Identifier: AGPL-3.0-only

// Package resources builds the Kubernetes objects a PaperlessInstance owns. Every
// builder is a pure function: instance in, object out, no client and no cluster
// access, so each is unit-testable without any Kubernetes at all.
package resources

import "github.com/p3l1/paperless-ngx-operator/api/v1alpha1"

// derivedName appends suffix to the instance name. Kubernetes caps every object
// name at 253 characters (RFC 1123 subdomain), a limit the instance's own name
// already satisfies on its own but a suffixed derived name can still exceed. A
// bare string has no channel to report that, so callers with an error return
// (SecretKey, AdminSecret) validate the result themselves; callers without one
// rely on the API server to reject an invalid name at apply time.
func derivedName(inst *v1alpha1.PaperlessInstance, suffix string) string {
	return inst.Name + "-" + suffix
}

// SecretKeyName is the Secret holding Django's PAPERLESS_SECRET_KEY.
func SecretKeyName(inst *v1alpha1.PaperlessInstance) string {
	return derivedName(inst, "secret-key")
}

// AdminSecretName is the Secret holding the local superuser's username and password.
func AdminSecretName(inst *v1alpha1.PaperlessInstance) string {
	return derivedName(inst, "admin")
}

// DataPVCName is the PersistentVolumeClaim for Paperless's own working state.
func DataPVCName(inst *v1alpha1.PaperlessInstance) string {
	return derivedName(inst, "data")
}

// MediaPVCName is the PersistentVolumeClaim for the document archive.
func MediaPVCName(inst *v1alpha1.PaperlessInstance) string {
	return derivedName(inst, "media")
}

// ConsumePVCName is the PersistentVolumeClaim for the watched inbox.
func ConsumePVCName(inst *v1alpha1.PaperlessInstance) string {
	return derivedName(inst, "consume")
}

// ExportPVCName is the PersistentVolumeClaim for document_exporter output.
func ExportPVCName(inst *v1alpha1.PaperlessInstance) string {
	return derivedName(inst, "export")
}

// ValkeyName names the managed cache's Deployment, Service and PersistentVolumeClaim.
func ValkeyName(inst *v1alpha1.PaperlessInstance) string {
	return derivedName(inst, "valkey")
}

// DBName is the CloudNativePG Cluster managing the instance's database.
func DBName(inst *v1alpha1.PaperlessInstance) string {
	return derivedName(inst, "db")
}
