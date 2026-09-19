// SPDX-License-Identifier: AGPL-3.0-only

// Package manager assembles the controller-runtime manager configuration.
package manager

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/p3l1/paperless-ngx-operator/api/v1alpha1"
)

// Changing this splits an upgrading operator into two active leaders.
const LeaderElectionID = "paperless-ngx-operator.paperless.p3l1.de"

type Config struct {
	MetricsAddress string
	ProbeAddress   string
	LeaderElection bool
}

func ControllerOptions(c Config) ctrl.Options {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))

	return ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: c.MetricsAddress},
		HealthProbeBindAddress: c.ProbeAddress,
		LeaderElection:         c.LeaderElection,
		LeaderElectionID:       LeaderElectionID,
	}
}
