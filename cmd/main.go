// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"flag"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/p3l1/paperless-ngx-operator/internal/manager"
	"github.com/p3l1/paperless-ngx-operator/internal/version"
)

func main() {
	cfg := manager.Config{}
	flag.StringVar(&cfg.MetricsAddress, "metrics-bind-address", ":8080", "address the metrics endpoint binds to")
	flag.StringVar(&cfg.ProbeAddress, "health-probe-bind-address", ":8081", "address the probe endpoint binds to")
	flag.BoolVar(&cfg.LeaderElection, "leader-elect", false, "elect a leader before acting")

	zapOpts := zap.Options{}
	zapOpts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))
	log := ctrl.Log.WithName("setup")
	log.Info("starting paperless-ngx-operator", "version", version.String())

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), manager.ControllerOptions(cfg))
	if err != nil {
		log.Error(err, "unable to create manager")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Error(err, "unable to register health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Error(err, "unable to register readiness check")
		os.Exit(1)
	}

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "manager exited with error")
		os.Exit(1)
	}
}
