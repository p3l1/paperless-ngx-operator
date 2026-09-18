// SPDX-License-Identifier: AGPL-3.0-only

package e2e

import (
	"context"
	"errors"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	kubeContext    = "k3d-paperless-operator"
	namespace      = "paperless-operator-system"
	deploymentName = "paperless-operator-paperless-ngx-operator"
)

var errNotAvailable = errors.New("deployment not available yet")

func newClient(t *testing.T) client.Client {
	t.Helper()

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{CurrentContext: kubeContext},
	).ClientConfig()
	if err != nil {
		t.Fatalf("no kubeconfig for context %s: %v (run: just cluster-up)", kubeContext, err)
	}

	c, err := client.New(cfg, client.Options{})
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	return c
}

// eachPoll retries fn every two seconds until it returns nil or the timeout expires.
func eachPoll(t *testing.T, timeout time.Duration, fn func() error) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		if last = fn(); last == nil {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("condition not met within %s: %v", timeout, last)
}

func TestOperatorDeploymentBecomesAvailable(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()

	eachPoll(t, 3*time.Minute, func() error {
		var d appsv1.Deployment
		key := types.NamespacedName{Namespace: namespace, Name: deploymentName}
		if err := c.Get(ctx, key, &d); err != nil {
			return err
		}
		for _, cond := range d.Status.Conditions {
			if cond.Type == appsv1.DeploymentAvailable && cond.Status == corev1.ConditionTrue {
				return nil
			}
		}
		return errNotAvailable
	})
}

func TestOperatorPodIsReadyWithoutRestarts(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()

	var pods corev1.PodList
	if err := c.List(ctx, &pods,
		client.InNamespace(namespace),
		client.MatchingLabels{"app.kubernetes.io/name": "paperless-ngx-operator"},
	); err != nil {
		t.Fatalf("listing operator pods: %v", err)
	}
	if len(pods.Items) != 1 {
		t.Fatalf("got %d operator pods, want 1", len(pods.Items))
	}

	pod := pods.Items[0]
	for _, cs := range pod.Status.ContainerStatuses {
		if !cs.Ready {
			t.Errorf("container %s is not ready", cs.Name)
		}
		if cs.RestartCount != 0 {
			t.Errorf("container %s restarted %d times; probes or image are wrong",
				cs.Name, cs.RestartCount)
		}
	}
}
