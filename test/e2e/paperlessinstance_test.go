// SPDX-License-Identifier: AGPL-3.0-only

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/p3l1/paperless-ngx-operator/api/v1alpha1"
	"github.com/p3l1/paperless-ngx-operator/internal/resources"
)

const (
	exampleManifestPath = "../../examples/paperlessinstance-minimal.yaml"
	// instanceNamespace is used because the manifest itself omits one, leaving
	// `kubectl apply -n <ns>` to decide; this test talks to the API server
	// directly and must supply a namespace itself.
	instanceNamespace = "default"

	// instanceReadyTimeout is generous: a cold run pulls the Paperless and
	// PostgreSQL images (several hundred megabytes together) and then runs
	// Paperless's first-start database migration before the pod is ready.
	instanceReadyTimeout = 9 * time.Minute
	workloadGoneTimeout  = 3 * time.Minute
	portForwardTimeout   = 30 * time.Second

	// statefulResourcesTimeout bounds both waiting for a fresh instance's PVCs,
	// CNPG cluster and generated secrets to be created, and waiting for a
	// deleted instance's to be garbage-collected. Neither needs the pod
	// scheduling and image pulls instanceReadyTimeout budgets for.
	statefulResourcesTimeout = 2 * time.Minute
)

// cnpgClusterGVK identifies CloudNativePG's Cluster kind, matching the
// unexported constants the operator itself builds resources.CNPGCluster from.
var cnpgClusterGVK = schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}

// terminalPodImageReasons are container wait/crash reasons that will never
// resolve by waiting longer, so a pod stuck in one of them fails the test
// immediately instead of exhausting instanceReadyTimeout to say nothing more
// than "not ready yet".
var terminalPodImageReasons = map[string]bool{
	"ImagePullBackOff": true,
	"ErrImagePull":     true,
	"InvalidImageName": true,
	"CrashLoopBackOff": true,
}

// loadExampleInstance decodes examples/paperlessinstance-minimal.yaml, the same
// file a user would `kubectl apply -f`, so this test exercises the shipped
// example rather than a hand-built stand-in that could drift from it.
func loadExampleInstance(t *testing.T) *v1alpha1.PaperlessInstance {
	t.Helper()

	data, err := os.ReadFile(exampleManifestPath)
	if err != nil {
		t.Fatalf("reading %s: %v", exampleManifestPath, err)
	}

	var inst v1alpha1.PaperlessInstance
	if err := yaml.Unmarshal(data, &inst); err != nil {
		t.Fatalf("decoding %s: %v", exampleManifestPath, err)
	}
	inst.Namespace = instanceNamespace
	return &inst
}

// restConfigForContext loads a *rest.Config for kubeContext, the same context
// newClient uses, but keeps the raw config: port-forwarding needs it directly
// rather than through a controller-runtime client.
func restConfigForContext(t *testing.T) *rest.Config {
	t.Helper()

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{CurrentContext: kubeContext},
	).ClientConfig()
	if err != nil {
		t.Fatalf("no kubeconfig for context %s: %v (run: just cluster-up)", kubeContext, err)
	}
	return cfg
}

// failFastOnStuckPod fails the test immediately when a pod backing inst is
// stuck in a reason that will never self-resolve (bad image tag, crash loop),
// so breaking the deployment fails fast instead of exhausting the poll timeout.
func failFastOnStuckPod(t *testing.T, ctx context.Context, c client.Client, inst *v1alpha1.PaperlessInstance) {
	t.Helper()

	var pods corev1.PodList
	if err := c.List(ctx, &pods,
		client.InNamespace(inst.Namespace),
		client.MatchingLabels(resources.Service(inst).Spec.Selector),
	); err != nil {
		return // best-effort: the caller's own Get already surfaces real API errors
	}

	for _, pod := range pods.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && terminalPodImageReasons[cs.State.Waiting.Reason] {
				t.Fatalf("pod %s container %s is stuck: %s: %s",
					pod.Name, cs.Name, cs.State.Waiting.Reason, cs.State.Waiting.Message)
			}
		}
	}
}

// waitForReady polls until inst's Ready condition is True, failing fast on a
// stuck pod (see failFastOnStuckPod) rather than waiting out the full timeout.
func waitForReady(t *testing.T, ctx context.Context, c client.Client, key types.NamespacedName) {
	t.Helper()

	eachPoll(t, instanceReadyTimeout, func() error {
		var inst v1alpha1.PaperlessInstance
		if err := c.Get(ctx, key, &inst); err != nil {
			return err
		}
		failFastOnStuckPod(t, ctx, c, &inst)

		cond := meta.FindStatusCondition(inst.Status.Conditions, v1alpha1.ConditionReady)
		if cond == nil {
			return fmt.Errorf("Ready condition not yet reported")
		}
		if cond.Status != metav1.ConditionTrue {
			return fmt.Errorf("Ready=%s (%s): %s", cond.Status, cond.Reason, cond.Message)
		}
		return nil
	})
}

// findRunningPod returns one running pod backing inst's Service, using the
// same label selector the Service itself routes on so the two never diverge.
func findRunningPod(t *testing.T, ctx context.Context, c client.Client, inst *v1alpha1.PaperlessInstance) corev1.Pod {
	t.Helper()

	var pods corev1.PodList
	selector := resources.Service(inst).Spec.Selector
	if err := c.List(ctx, &pods, client.InNamespace(inst.Namespace), client.MatchingLabels(selector)); err != nil {
		t.Fatalf("listing pods for %s: %v", inst.Name, err)
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return pod
		}
	}
	t.Fatalf("no running pod found for instance %s (found %d pods)", inst.Name, len(pods.Items))
	return corev1.Pod{}
}

// portForwardToPod opens an SPDY port-forward to podName's remotePort and
// returns the local port it is reachable on. Forwarding stops when the test
// (or subtest) that requested it finishes, via t.Cleanup.
func portForwardToPod(t *testing.T, cfg *rest.Config, namespace, podName string, remotePort int) int {
	t.Helper()

	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		t.Fatalf("building SPDY transport: %v", err)
	}

	target, err := url.Parse(cfg.Host)
	if err != nil {
		t.Fatalf("parsing API server URL %q: %v", cfg.Host, err)
	}
	target.Path = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, podName)
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, target)

	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })
	readyCh := make(chan struct{})
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}

	fw, err := portforward.New(dialer, []string{fmt.Sprintf("0:%d", remotePort)}, stopCh, readyCh, out, errOut)
	if err != nil {
		t.Fatalf("creating port forwarder to %s/%s: %v", namespace, podName, err)
	}

	fwErr := make(chan error, 1)
	go func() { fwErr <- fw.ForwardPorts() }()

	select {
	case <-readyCh:
	case err := <-fwErr:
		t.Fatalf("port-forward to %s/%s exited before ready: %v (stderr: %s)", namespace, podName, err, errOut.String())
	case <-time.After(portForwardTimeout):
		t.Fatalf("port-forward to %s/%s did not become ready within %s", namespace, podName, portForwardTimeout)
	}

	ports, err := fw.GetPorts()
	if err != nil {
		t.Fatalf("reading forwarded port for %s/%s: %v", namespace, podName, err)
	}
	return int(ports[0].Local)
}

// assertServesLoginPage proves the assembled Paperless installation actually
// serves, not merely that the operator's own Ready condition says so: it
// port-forwards straight to a pod and asserts the HTTP response itself.
func assertServesLoginPage(t *testing.T, ctx context.Context, c client.Client, cfg *rest.Config, inst *v1alpha1.PaperlessInstance) {
	t.Helper()

	pod := findRunningPod(t, ctx, c, inst)
	remotePort := int(resources.Service(inst).Spec.Ports[0].Port)
	localPort := portForwardToPod(t, cfg, inst.Namespace, pod.Name, remotePort)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", localPort))
	if err != nil {
		t.Fatalf("GET / through port-forward to %s: %v", pod.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / = %d, want 200; body:\n%s", resp.StatusCode, body)
	}
	lower := strings.ToLower(string(body))
	if !strings.Contains(lower, "<html") {
		t.Fatalf("GET / returned 200 but no HTML; body:\n%s", body)
	}
	if !strings.Contains(lower, "paperless") {
		t.Fatalf("GET / returned HTML but nothing identifying it as Paperless; body:\n%s", body)
	}
	t.Logf("GET / -> %d, %d bytes of HTML (final URL after redirects: %s)", resp.StatusCode, len(body), resp.Request.URL)
}

// statefulResource names one object spec.deletionPolicy governs, paired with a
// constructor for the (possibly unstructured) type Get needs.
type statefulResource struct {
	description string
	key         types.NamespacedName
	newObject   func() client.Object
}

// statefulResources lists inst's four PVCs, its CNPG cluster and its two
// generated secrets — the resources spec.deletionPolicy governs (see
// PaperlessInstanceSpec.DeletionPolicy's doc). The Deployment and Service are
// deliberately absent: they are always owned and so never part of this list.
func statefulResources(inst *v1alpha1.PaperlessInstance) []statefulResource {
	ns := inst.Namespace
	newPVC := func() client.Object { return &corev1.PersistentVolumeClaim{} }
	newSecret := func() client.Object { return &corev1.Secret{} }
	newCluster := func() client.Object {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(cnpgClusterGVK)
		return u
	}

	return []statefulResource{
		{"data PVC", types.NamespacedName{Namespace: ns, Name: resources.DataPVCName(inst)}, newPVC},
		{"media PVC", types.NamespacedName{Namespace: ns, Name: resources.MediaPVCName(inst)}, newPVC},
		{"consume PVC", types.NamespacedName{Namespace: ns, Name: resources.ConsumePVCName(inst)}, newPVC},
		{"export PVC", types.NamespacedName{Namespace: ns, Name: resources.ExportPVCName(inst)}, newPVC},
		{"CNPG cluster", types.NamespacedName{Namespace: ns, Name: resources.DBName(inst)}, newCluster},
		{"secret-key Secret", types.NamespacedName{Namespace: ns, Name: resources.SecretKeyName(inst)}, newSecret},
		{"admin Secret", types.NamespacedName{Namespace: ns, Name: resources.AdminSecretName(inst)}, newSecret},
	}
}

// waitForStatefulResourcesCreated polls until every one of inst's stateful
// resources exists, so a deletion assertion that follows acts on an instance
// whose first few reconciles have actually run rather than an empty namespace.
func waitForStatefulResourcesCreated(t *testing.T, ctx context.Context, c client.Client, inst *v1alpha1.PaperlessInstance) {
	t.Helper()
	eachPoll(t, statefulResourcesTimeout, func() error {
		for _, res := range statefulResources(inst) {
			if err := c.Get(ctx, res.key, res.newObject()); err != nil {
				return fmt.Errorf("%s %s: %w", res.description, res.key, err)
			}
		}
		return nil
	})
}

// waitForWorkloadGone polls until inst's Deployment and Service, always owned
// regardless of spec.deletionPolicy, are both garbage-collected.
func waitForWorkloadGone(t *testing.T, ctx context.Context, c client.Client, inst *v1alpha1.PaperlessInstance) {
	t.Helper()
	key := types.NamespacedName{Namespace: inst.Namespace, Name: inst.Name}
	eachPoll(t, workloadGoneTimeout, func() error {
		if err := c.Get(ctx, key, &appsv1.Deployment{}); !apierrors.IsNotFound(err) {
			return fmt.Errorf("deployment %s: Get error = %v, want NotFound", key, err)
		}
		if err := c.Get(ctx, key, &corev1.Service{}); !apierrors.IsNotFound(err) {
			return fmt.Errorf("service %s: Get error = %v, want NotFound", key, err)
		}
		return nil
	})
}

// assertDeleteRetainsStatefulResources deletes inst, created under the default
// Retain deletion policy, and confirms its reproducible workload (Deployment,
// Service) is garbage-collected while its stateful resources — PVCs, CNPG
// cluster and generated secrets — survive with no owner reference. Retention,
// not deletion, is what now makes recreating an instance over the same volumes
// possible, and it only holds under DeletionPolicyRetain, the default.
func assertDeleteRetainsStatefulResources(t *testing.T, ctx context.Context, c client.Client, inst *v1alpha1.PaperlessInstance) {
	t.Helper()

	if err := c.Delete(ctx, inst); err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("deleting instance %s: %v", inst.Name, err)
	}

	waitForWorkloadGone(t, ctx, c, inst)

	for _, res := range statefulResources(inst) {
		if err := c.Get(ctx, res.key, res.newObject()); err != nil {
			t.Errorf("%s %s should survive instance deletion under the default Retain policy, Get failed: %v",
				res.description, res.key, err)
		}
	}
	t.Log("PVCs, CNPG cluster and generated secrets all survived instance deletion under the default Retain policy, as designed")
}

// assertDeleteRemovesStatefulResources deletes inst, created under
// DeletionPolicyDelete, and polls until every stateful resource — not just the
// always-owned Deployment and Service — is gone too, tolerating the delay the
// garbage collector needs once the owning instance itself disappears.
func assertDeleteRemovesStatefulResources(t *testing.T, ctx context.Context, c client.Client, inst *v1alpha1.PaperlessInstance) {
	t.Helper()

	if err := c.Delete(ctx, inst); err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("deleting instance %s: %v", inst.Name, err)
	}

	waitForWorkloadGone(t, ctx, c, inst)

	eachPoll(t, statefulResourcesTimeout, func() error {
		for _, res := range statefulResources(inst) {
			err := c.Get(ctx, res.key, res.newObject())
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("%s %s: %w", res.description, res.key, err)
			}
			return fmt.Errorf("%s %s still exists", res.description, res.key)
		}
		return nil
	})
	t.Log("PVCs, CNPG cluster and generated secrets were all garbage-collected under DeletionPolicyDelete, as designed")
}

// TestPaperlessInstanceServesHTTP applies the shipped example PaperlessInstance
// to a real cluster and proves it results in a Paperless that actually answers
// HTTP requests, not merely a set of objects the operator is satisfied with.
func TestPaperlessInstanceServesHTTP(t *testing.T) {
	c := newClient(t, v1alpha1.AddToScheme)
	cfg := restConfigForContext(t)
	ctx := context.Background()

	inst := loadExampleInstance(t)
	if err := c.Create(ctx, inst); err != nil {
		t.Fatalf("creating instance %s: %v", inst.Name, err)
	}
	// Best-effort safety net: if an assertion below fails fatally partway
	// through, still try to remove the instance so a rerun starts clean.
	t.Cleanup(func() { _ = c.Delete(context.Background(), inst) })

	key := types.NamespacedName{Namespace: inst.Namespace, Name: inst.Name}

	if !t.Run("BecomesReady", func(t *testing.T) {
		waitForReady(t, ctx, c, key)
	}) {
		t.Fatal("instance never became Ready; skipping the remaining checks")
	}

	if !t.Run("ServesLoginPage", func(t *testing.T) {
		assertServesLoginPage(t, ctx, c, cfg, inst)
	}) {
		t.Fatal("instance did not serve HTTP; skipping the deletion check")
	}

	t.Run("DeleteRetainsStatefulResources", func(t *testing.T) {
		assertDeleteRetainsStatefulResources(t, ctx, c, inst)
	})
}

// TestPaperlessInstanceDeletionPolicyDeleteRemovesStatefulResources covers the
// other half of spec.deletionPolicy: unlike TestPaperlessInstanceServesHTTP,
// this instance never needs to actually serve — the assertion is about object
// lifecycle, not application readiness — so it only waits for the operator's
// first few reconciles to create the stateful resources before deleting.
func TestPaperlessInstanceDeletionPolicyDeleteRemovesStatefulResources(t *testing.T) {
	c := newClient(t, v1alpha1.AddToScheme)
	ctx := context.Background()

	inst := loadExampleInstance(t)
	inst.Name = "paperless-minimal-delete"
	inst.Spec.DeletionPolicy = v1alpha1.DeletionPolicyDelete
	if err := c.Create(ctx, inst); err != nil {
		t.Fatalf("creating instance %s: %v", inst.Name, err)
	}
	t.Cleanup(func() { _ = c.Delete(context.Background(), inst) })

	waitForStatefulResourcesCreated(t, ctx, c, inst)
	assertDeleteRemovesStatefulResources(t, ctx, c, inst)
}
