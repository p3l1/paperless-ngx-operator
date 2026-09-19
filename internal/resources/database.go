// SPDX-License-Identifier: AGPL-3.0-only

package resources

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/p3l1/paperless-ngx-operator/api/v1alpha1"
)

// managedDatabaseName is the database (and, since CNPG's initdb.owner defaults to it,
// the user) a managed CNPG cluster bootstraps. It is not user-configurable:
// ManagedDatabase carries no name field, unlike ExternalDatabase.
const managedDatabaseName = "paperless"

// cnpgAPIVersion and cnpgKind identify the CloudNativePG Cluster this package builds as
// unstructured.Unstructured, so the operator never imports CloudNativePG's API module
// (which would make an optional dependency a hard one for every user).
const (
	cnpgAPIVersion = "postgresql.cnpg.io/v1"
	cnpgKind       = "Cluster"
)

// cnpgAppSecretName is the credentials Secret CNPG generates for the application user,
// following CNPG's own "<cluster name>-app" naming convention.
func cnpgAppSecretName(inst *v1alpha1.PaperlessInstance) string {
	return DBName(inst) + "-app"
}

// cnpgServiceName is the read-write Service CNPG generates for the primary instance,
// following CNPG's own "<cluster name>-rw" naming convention.
func cnpgServiceName(inst *v1alpha1.PaperlessInstance) string {
	return DBName(inst) + "-rw"
}

// secretEnvVar builds an EnvVar sourced from a key in a Secret.
func secretEnvVar(name, secretName, key string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  key,
			},
		},
	}
}

// DatabaseEnv returns the PAPERLESS_DB* environment variables Paperless needs to reach
// its database, branching on whether it is managed or external.
//
// PAPERLESS_DBENGINE is always "postgresql": leaving it unset makes Paperless fall back
// to SQLite silently, which is wrong for both branches.
func DatabaseEnv(inst *v1alpha1.PaperlessInstance) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{Name: "PAPERLESS_DBENGINE", Value: "postgresql"},
	}

	if inst.Spec.Database.IsExternal() {
		ext := inst.Spec.Database.External
		return append(env,
			corev1.EnvVar{Name: "PAPERLESS_DBHOST", Value: ext.Host},
			corev1.EnvVar{Name: "PAPERLESS_DBPORT", Value: strconv.Itoa(int(ext.Port))},
			corev1.EnvVar{Name: "PAPERLESS_DBNAME", Value: ext.Name},
			secretEnvVar("PAPERLESS_DBUSER", ext.CredentialsSecretRef.Name, "username"),
			secretEnvVar("PAPERLESS_DBPASS", ext.CredentialsSecretRef.Name, "password"),
		)
	}

	secretName := cnpgAppSecretName(inst)
	return append(env,
		corev1.EnvVar{Name: "PAPERLESS_DBHOST", Value: cnpgServiceName(inst)},
		corev1.EnvVar{Name: "PAPERLESS_DBPORT", Value: "5432"},
		corev1.EnvVar{Name: "PAPERLESS_DBNAME", Value: managedDatabaseName},
		secretEnvVar("PAPERLESS_DBUSER", secretName, "username"),
		secretEnvVar("PAPERLESS_DBPASS", secretName, "password"),
	)
}

// CNPGCluster returns the CloudNativePG Cluster for a managed database, built as
// unstructured.Unstructured so the operator has no hard dependency on CloudNativePG's
// API module — it must keep working in clusters where CNPG is absent.
//
// Field paths (spec.instances, spec.storage.size, spec.storage.storageClass,
// spec.bootstrap.initdb.database) were verified with `kubectl explain` and a
// server-side dry-run apply against a live CloudNativePG installation.
func CNPGCluster(inst *v1alpha1.PaperlessInstance) (*unstructured.Unstructured, error) {
	name := DBName(inst)
	if err := validateName(name); err != nil {
		return nil, err
	}

	cnpg := inst.Spec.Database.CNPG
	if cnpg == nil {
		cnpg = &v1alpha1.ManagedDatabase{}
	}

	instances := cnpg.Instances
	if instances < 1 {
		instances = 1
	}

	size := cnpg.Storage.SizeOrDefault(v1alpha1.DefaultDatabaseSize)
	storage := map[string]interface{}{
		"size": size.String(),
	}
	if cnpg.Storage.StorageClassName != nil {
		storage["storageClass"] = *cnpg.Storage.StorageClassName
	}

	spec := map[string]interface{}{
		"instances": int64(instances),
		"storage":   storage,
		"bootstrap": map[string]interface{}{
			"initdb": map[string]interface{}{
				"database": managedDatabaseName,
			},
		},
	}

	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(cnpgAPIVersion)
	obj.SetKind(cnpgKind)
	obj.SetName(name)
	obj.SetNamespace(inst.Namespace)
	obj.SetLabels(commonLabels(inst, "database"))

	if err := unstructured.SetNestedMap(obj.Object, spec, "spec"); err != nil {
		return nil, fmt.Errorf("setting cluster spec: %w", err)
	}
	return obj, nil
}
