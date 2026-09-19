// SPDX-License-Identifier: AGPL-3.0-only

package manager

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestControllerOptionsPassesAddressesThrough(t *testing.T) {
	opts := ControllerOptions(Config{
		MetricsAddress: ":9090",
		ProbeAddress:   ":9091",
		LeaderElection: true,
	})

	if got, want := opts.Metrics.BindAddress, ":9090"; got != want {
		t.Errorf("metrics address = %q, want %q", got, want)
	}
	if got, want := opts.HealthProbeBindAddress, ":9091"; got != want {
		t.Errorf("probe address = %q, want %q", got, want)
	}
	if !opts.LeaderElection {
		t.Error("leader election disabled, want enabled")
	}
}

func TestControllerOptionsUsesStableLeaderElectionID(t *testing.T) {
	opts := ControllerOptions(Config{})

	if got, want := opts.LeaderElectionID, LeaderElectionID; got != want {
		t.Errorf("lock name = %q, want %q", got, want)
	}
	if want := "paperless-ngx-operator.paperless.p3l1.de"; LeaderElectionID != want {
		t.Errorf("LeaderElectionID = %q, want %q", LeaderElectionID, want)
	}
}

func TestControllerOptionsSchemeKnowsCoreTypes(t *testing.T) {
	opts := ControllerOptions(Config{})

	if !opts.Scheme.Recognizes(corev1.SchemeGroupVersion.WithKind("Secret")) {
		t.Error("scheme does not recognise core/v1 Secret")
	}
}

// TestControllerOptionsDisablesTheSecretCache guards against the client
// lazily starting a cluster-wide Secret informer: the reconciler only ever
// Gets two Secrets by name, so caching every Secret in the cluster in the
// operator's own memory is unnecessary exposure, not a performance choice.
func TestControllerOptionsDisablesTheSecretCache(t *testing.T) {
	opts := ControllerOptions(Config{})

	if opts.Client.Cache == nil {
		t.Fatal("Client.Cache is nil, want Secret excluded from caching")
	}
	for _, obj := range opts.Client.Cache.DisableFor {
		if _, ok := obj.(*corev1.Secret); ok {
			return
		}
	}
	t.Error("Client.Cache.DisableFor does not exclude corev1.Secret")
}
