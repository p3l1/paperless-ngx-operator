// SPDX-License-Identifier: AGPL-3.0-only

package envtest_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/p3l1/paperless-ngx-operator/test/envtestenv"
)

func TestEnvironmentServesCoreAPI(t *testing.T) {
	c, ctx := envtestenv.Start(t)

	var namespaces corev1.NamespaceList
	if err := c.List(ctx, &namespaces); err != nil {
		t.Fatalf("listing namespaces: %v", err)
	}
	if len(namespaces.Items) == 0 {
		t.Error("no namespaces returned; the API server is not serving core types")
	}
}
