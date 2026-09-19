// SPDX-License-Identifier: AGPL-3.0-only

package resources

import (
	"strconv"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/p3l1/paperless-ngx-operator/api/v1alpha1"
)

// envValue returns the literal Value of the named EnvVar, failing the test if it is absent
// or sourced from a secret instead.
func envValue(t *testing.T, env []corev1.EnvVar, name string) string {
	t.Helper()
	for _, e := range env {
		if e.Name == name {
			if e.ValueFrom != nil {
				t.Fatalf("%s is sourced via ValueFrom, want a literal Value", name)
			}
			return e.Value
		}
	}
	t.Fatalf("no env var named %q among %d entries", name, len(env))
	return ""
}

// envSecretRef returns the secret name and key the named EnvVar reads from, failing the
// test if it is absent or set as a literal instead.
func envSecretRef(t *testing.T, env []corev1.EnvVar, name string) (secretName, key string) {
	t.Helper()
	for _, e := range env {
		if e.Name == name {
			if e.ValueFrom == nil || e.ValueFrom.SecretKeyRef == nil {
				t.Fatalf("%s is not sourced from a SecretKeyRef", name)
			}
			return e.ValueFrom.SecretKeyRef.Name, e.ValueFrom.SecretKeyRef.Key
		}
	}
	t.Fatalf("no env var named %q among %d entries", name, len(env))
	return "", ""
}

func TestDatabaseEnvSetsPostgresEngineForManaged(t *testing.T) {
	inst := instance() // spec.database is unset, which is the "managed" branch

	if got := envValue(t, DatabaseEnv(inst), "PAPERLESS_DBENGINE"); got != "postgresql" {
		t.Errorf("PAPERLESS_DBENGINE = %q, want %q", got, "postgresql")
	}
}

func TestDatabaseEnvManagedPointsAtCNPGService(t *testing.T) {
	inst := instance()
	inst.Spec.Database.CNPG = &v1alpha1.ManagedDatabase{Instances: 1}

	env := DatabaseEnv(inst)

	wantHost := DBName(inst) + "-rw"
	if got := envValue(t, env, "PAPERLESS_DBHOST"); got != wantHost {
		t.Errorf("PAPERLESS_DBHOST = %q, want %q", got, wantHost)
	}

	wantSecret := DBName(inst) + "-app"
	if secretName, key := envSecretRef(t, env, "PAPERLESS_DBUSER"); secretName != wantSecret || key != "username" {
		t.Errorf("PAPERLESS_DBUSER = secret %q key %q, want secret %q key %q", secretName, key, wantSecret, "username")
	}
	if secretName, key := envSecretRef(t, env, "PAPERLESS_DBPASS"); secretName != wantSecret || key != "password" {
		t.Errorf("PAPERLESS_DBPASS = secret %q key %q, want secret %q key %q", secretName, key, wantSecret, "password")
	}
}

func TestDatabaseEnvExternalReadsHostPortNameFromSpec(t *testing.T) {
	inst := instance()
	inst.Spec.Database.External = &v1alpha1.ExternalDatabase{
		Host: "pg.example.org",
		Port: 15432, // a deliberately non-default port
		Name: "paperless_prod",
		CredentialsSecretRef: corev1.LocalObjectReference{
			Name: "external-db-creds",
		},
	}

	env := DatabaseEnv(inst)

	if got := envValue(t, env, "PAPERLESS_DBENGINE"); got != "postgresql" {
		t.Errorf("PAPERLESS_DBENGINE = %q, want %q", got, "postgresql")
	}
	if got := envValue(t, env, "PAPERLESS_DBHOST"); got != "pg.example.org" {
		t.Errorf("PAPERLESS_DBHOST = %q, want %q", got, "pg.example.org")
	}
	if got, want := envValue(t, env, "PAPERLESS_DBPORT"), strconv.Itoa(15432); got != want {
		t.Errorf("PAPERLESS_DBPORT = %q, want %q", got, want)
	}
	if got := envValue(t, env, "PAPERLESS_DBNAME"); got != "paperless_prod" {
		t.Errorf("PAPERLESS_DBNAME = %q, want %q", got, "paperless_prod")
	}
	if secretName, key := envSecretRef(t, env, "PAPERLESS_DBUSER"); secretName != "external-db-creds" || key != "username" {
		t.Errorf("PAPERLESS_DBUSER = secret %q key %q, want secret %q key %q", secretName, key, "external-db-creds", "username")
	}
	if secretName, key := envSecretRef(t, env, "PAPERLESS_DBPASS"); secretName != "external-db-creds" || key != "password" {
		t.Errorf("PAPERLESS_DBPASS = secret %q key %q, want secret %q key %q", secretName, key, "external-db-creds", "password")
	}
}

// Both database.cnpg and database.external are meaningless to set together, but the CEL
// rule that rejects it lives on the type, not here; IsExternal() must still resolve one
// unambiguous branch so a raw struct bypassing admission doesn't panic or emit both.
func TestDatabaseEnvBothSetPrefersExternal(t *testing.T) {
	inst := instance()
	inst.Spec.Database.CNPG = &v1alpha1.ManagedDatabase{Instances: 3}
	inst.Spec.Database.External = &v1alpha1.ExternalDatabase{
		Host: "pg.example.org",
		Name: "paperless",
		CredentialsSecretRef: corev1.LocalObjectReference{
			Name: "external-db-creds",
		},
	}

	env := DatabaseEnv(inst)
	if got := envValue(t, env, "PAPERLESS_DBHOST"); got != "pg.example.org" {
		t.Errorf("PAPERLESS_DBHOST = %q, want the external host %q", got, "pg.example.org")
	}
}

func TestCNPGClusterSetsInstancesAndBootstrapDatabase(t *testing.T) {
	inst := instance()
	inst.Spec.Database.CNPG = &v1alpha1.ManagedDatabase{Instances: 3}

	obj, err := CNPGCluster(inst)
	if err != nil {
		t.Fatalf("CNPGCluster: %v", err)
	}

	got, found, err := unstructured.NestedInt64(obj.Object, "spec", "instances")
	if err != nil || !found {
		t.Fatalf("spec.instances: found=%v err=%v", found, err)
	}
	if got != 3 {
		t.Errorf("spec.instances = %d, want 3", got)
	}

	db, found, err := unstructured.NestedString(obj.Object, "spec", "bootstrap", "initdb", "database")
	if err != nil || !found {
		t.Fatalf("spec.bootstrap.initdb.database: found=%v err=%v", found, err)
	}
	if db == "" {
		t.Error("spec.bootstrap.initdb.database is empty")
	}

	if got, want := obj.GetAPIVersion(), "postgresql.cnpg.io/v1"; got != want {
		t.Errorf("apiVersion = %q, want %q", got, want)
	}
	if got, want := obj.GetKind(), "Cluster"; got != want {
		t.Errorf("kind = %q, want %q", got, want)
	}
	if got, want := obj.GetName(), DBName(inst); got != want {
		t.Errorf("name = %q, want %q", got, want)
	}
}

// A bare struct (no admission, no kubebuilder default) must not crash or produce an
// invalid Cluster: CNPG itself rejects spec.instances < 1.
func TestCNPGClusterDefaultsInstancesWhenUnset(t *testing.T) {
	inst := instance() // spec.database.cnpg is nil entirely

	obj, err := CNPGCluster(inst)
	if err != nil {
		t.Fatalf("CNPGCluster: %v", err)
	}

	got, found, err := unstructured.NestedInt64(obj.Object, "spec", "instances")
	if err != nil || !found {
		t.Fatalf("spec.instances: found=%v err=%v", found, err)
	}
	if got < 1 {
		t.Errorf("spec.instances = %d, want at least 1", got)
	}
}

func TestCNPGClusterStorageSizeDefaultsWhenUnset(t *testing.T) {
	inst := instance()
	inst.Spec.Database.CNPG = &v1alpha1.ManagedDatabase{Instances: 1}

	obj, err := CNPGCluster(inst)
	if err != nil {
		t.Fatalf("CNPGCluster: %v", err)
	}

	sizeStr, found, err := unstructured.NestedString(obj.Object, "spec", "storage", "size")
	if err != nil || !found {
		t.Fatalf("spec.storage.size: found=%v err=%v", found, err)
	}
	got := resource.MustParse(sizeStr)
	want := resource.MustParse(v1alpha1.DefaultDatabaseSize)
	if got.Cmp(want) != 0 {
		t.Errorf("spec.storage.size = %q, want %q", sizeStr, v1alpha1.DefaultDatabaseSize)
	}
}

func TestCNPGClusterStorageSizeFromSpec(t *testing.T) {
	inst := instance()
	inst.Spec.Database.CNPG = &v1alpha1.ManagedDatabase{
		Instances: 1,
		Storage:   v1alpha1.VolumeSpec{Size: resource.MustParse("42Gi")},
	}

	obj, err := CNPGCluster(inst)
	if err != nil {
		t.Fatalf("CNPGCluster: %v", err)
	}

	sizeStr, _, _ := unstructured.NestedString(obj.Object, "spec", "storage", "size")
	got := resource.MustParse(sizeStr)
	want := resource.MustParse("42Gi")
	if got.Cmp(want) != 0 {
		t.Errorf("spec.storage.size = %q, want %q", sizeStr, "42Gi")
	}
}

func TestCNPGClusterStorageClassNameUnsetIsAbsent(t *testing.T) {
	inst := instance()
	inst.Spec.Database.CNPG = &v1alpha1.ManagedDatabase{Instances: 1}

	obj, err := CNPGCluster(inst)
	if err != nil {
		t.Fatalf("CNPGCluster: %v", err)
	}

	if _, found, _ := unstructured.NestedString(obj.Object, "spec", "storage", "storageClass"); found {
		t.Error("spec.storage.storageClass is present, want absent when unset")
	}
}

func TestCNPGClusterStorageClassNameExplicitEmptyIsPreserved(t *testing.T) {
	inst := instance()
	empty := ""
	inst.Spec.Database.CNPG = &v1alpha1.ManagedDatabase{
		Instances: 1,
		Storage:   v1alpha1.VolumeSpec{StorageClassName: &empty},
	}

	obj, err := CNPGCluster(inst)
	if err != nil {
		t.Fatalf("CNPGCluster: %v", err)
	}

	got, found, err := unstructured.NestedString(obj.Object, "spec", "storage", "storageClass")
	if err != nil || !found {
		t.Fatalf("spec.storage.storageClass: found=%v err=%v", found, err)
	}
	if got != "" {
		t.Errorf("spec.storage.storageClass = %q, want empty string", got)
	}
}

func TestCNPGClusterStorageClassNameSetIsPassedThrough(t *testing.T) {
	inst := instance()
	class := "fast-ssd"
	inst.Spec.Database.CNPG = &v1alpha1.ManagedDatabase{
		Instances: 1,
		Storage:   v1alpha1.VolumeSpec{StorageClassName: &class},
	}

	obj, err := CNPGCluster(inst)
	if err != nil {
		t.Fatalf("CNPGCluster: %v", err)
	}

	got, _, _ := unstructured.NestedString(obj.Object, "spec", "storage", "storageClass")
	if got != "fast-ssd" {
		t.Errorf("spec.storage.storageClass = %q, want %q", got, "fast-ssd")
	}
}

// Mirrors TestSecretBuildersRejectNameTooLong: the Cluster name is derived the same way
// generated Secret names are, so it can exceed the API server's 253-character ceiling too.
func TestCNPGClusterRejectsNameTooLong(t *testing.T) {
	inst := instance()
	// DBName appends "-db" (3 chars); 250 alone would land exactly at the 253-character
	// ceiling, so this needs a few characters more to genuinely exceed it.
	inst.Name = strings.Repeat("a", 252)

	if _, err := CNPGCluster(inst); err == nil {
		t.Error("CNPGCluster: want error for a name exceeding 253 characters, got nil")
	}
}
