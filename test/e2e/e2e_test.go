// SPDX-License-Identifier: AGPL-3.0-only

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	kubeContext    = "k3d-paperless-operator"
	namespace      = "paperless-operator-system"
	deploymentName = "paperless-operator-paperless-ngx-operator"
	readyTimeout   = 3 * time.Minute
)

// newClient builds a client for kubeContext. addToScheme registers extra API
// types (e.g. a CRD) on top of the built-in client-go scheme for callers that need them.
func newClient(t *testing.T, addToScheme ...func(*runtime.Scheme) error) client.Client {
	t.Helper()

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{CurrentContext: kubeContext},
	).ClientConfig()
	if err != nil {
		t.Fatalf("no kubeconfig for context %s: %v (run: just cluster-up)", kubeContext, err)
	}

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
	return c
}

// eachPoll always calls fn at least once, even for a non-positive timeout, so a
// caller can't mistake "never checked" for "checked and failed".
func eachPoll(t *testing.T, timeout time.Duration, fn func() error) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		err := fn()
		if err == nil {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("condition not met within %s: %v", timeout, err)
		}
		time.Sleep(2 * time.Second)
	}
}

func TestOperatorDeploymentBecomesAvailable(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()

	eachPoll(t, readyTimeout, func() error {
		var d appsv1.Deployment
		key := types.NamespacedName{Namespace: namespace, Name: deploymentName}
		if err := c.Get(ctx, key, &d); err != nil {
			return err
		}

		desired := int32(1)
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		ready := d.Status.ReadyReplicas
		// A Deployment's Available condition can hold true at zero desired replicas,
		// so readiness is judged from the replica counts, not that condition.
		if ready >= 1 && ready == desired {
			return nil
		}
		return fmt.Errorf("deployment not ready: %d/%d replicas ready", ready, desired)
	})
}

func TestOperatorPodIsReadyWithoutRestarts(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()

	var pod corev1.Pod
	eachPoll(t, readyTimeout, func() error {
		var pods corev1.PodList
		if err := c.List(ctx, &pods,
			client.InNamespace(namespace),
			client.MatchingLabels{"app.kubernetes.io/name": "paperless-ngx-operator"},
		); err != nil {
			return fmt.Errorf("listing operator pods: %w", err)
		}
		if len(pods.Items) != 1 {
			return fmt.Errorf("got %d operator pods, want 1", len(pods.Items))
		}
		for _, cs := range pods.Items[0].Status.ContainerStatuses {
			if !cs.Ready {
				return fmt.Errorf("container %s is not ready", cs.Name)
			}
		}
		pod = pods.Items[0]
		return nil
	})

	// Restart count is judged once, after the pod is found ready: a restart that
	// already happened must fail the test, not be retried away.
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.RestartCount != 0 {
			t.Errorf("container %s restarted %d times; probes or image are wrong",
				cs.Name, cs.RestartCount)
		}
	}
}
