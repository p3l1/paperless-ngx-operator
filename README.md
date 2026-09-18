# paperless-ngx-operator

A Kubernetes operator managing the lifecycle of [Paperless-NGX](https://docs.paperless-ngx.com)
instances: creation, e-mail integration, backup to S3 or a PVC, restore, and periodic proof
that a backup can actually be restored.

Status: early development. Slice 0 (foundation) is complete; no custom resources exist yet.

## Installation

No release has been published yet: the release workflow only runs on a `v*` tag, and none
has been cut. The command below is the intended interface and will work once a release
exists; today it fails with an OCI "not found" error.

    helm install paperless-operator \
      oci://ghcr.io/p3l1/charts/paperless-ngx-operator \
      --namespace paperless-operator-system --create-namespace

The chart uses no `lookup`, no random values and no hooks, so it renders identically under
ArgoCD, Flux or plain `helm install`. ArgoCD users should set `ServerSideApply=true`, because
the CRD schemas exceed the annotation size limit of client-side apply.

## Development

Requires Go, Docker, helm, kubectl, k3d and kubeconform in `PATH` — `just setup` fails if
any of those six is missing. `syft` (needed only by `just sbom`) and `helm-docs` (needed
only by `just docs`) get a warning instead, since nothing else depends on either.

    just setup        # verify tools, install the commit-msg hook
    just check        # fast: format, vet, lint, unit tests
    just test         # medium: envtest against a real API server
    just e2e          # full: k3d, chart install, assertions
    just --list       # everything else

## Licence

AGPL-3.0-only. See `LICENSE`.
