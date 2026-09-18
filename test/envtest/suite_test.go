// SPDX-License-Identifier: AGPL-3.0-only

package envtest_test

import (
	"context"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// startEnvironment boots an API server with the project's CRDs and stops it with the
// test. addToScheme registers extra API types (e.g. a CRD) on top of the built-in
// client-go scheme for callers that need them.
func startEnvironment(t *testing.T, addToScheme ...func(*runtime.Scheme) error) (client.Client, context.Context) {
	t.Helper()

	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: false,
	}

	cfg, err := env.Start()
	if err != nil {
		t.Fatalf("starting envtest: %v (run: just test)", err)
	}
	t.Cleanup(func() {
		if err := env.Stop(); err != nil {
			t.Errorf("stopping envtest: %v", err)
		}
	})

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("registering client-go scheme: %v", err)
	}
	for _, add := range addToScheme {
		if err := add(scheme); err != nil {
			t.Fatalf("registering scheme: %v", err)
		}
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	return c, context.Background()
}

func TestEnvironmentServesCoreAPI(t *testing.T) {
	c, ctx := startEnvironment(t)

	var namespaces corev1.NamespaceList
	if err := c.List(ctx, &namespaces); err != nil {
		t.Fatalf("listing namespaces: %v", err)
	}
	if len(namespaces.Items) == 0 {
		t.Error("no namespaces returned; the API server is not serving core types")
	}
}
