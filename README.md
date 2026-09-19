# paperless-ngx-operator

A Kubernetes operator managing the lifecycle of [Paperless-NGX](https://docs.paperless-ngx.com)
instances: creation, e-mail integration, backup to S3 or a PVC, restore, and periodic proof
that a backup can actually be restored.

Status: early development. The `PaperlessInstance` custom resource is available: applying
one creates a full Paperless-NGX deployment, with its database managed by
[CloudNativePG](https://cloudnative-pg.io) by default.

## Installation

No release has been published yet: the release workflow (see [Releasing](#releasing)) has
never been dispatched. The command below is the intended interface and will work once a
release exists; today it fails with an OCI "not found" error.

    helm install paperless-operator \
      oci://ghcr.io/p3l1/charts/paperless-ngx-operator \
      --namespace paperless-operator-system --create-namespace

The chart uses no `lookup`, no random values and no hooks, so it renders identically under
ArgoCD, Flux or plain `helm install`. ArgoCD users should set `ServerSideApply=true`, because
the CRD schemas exceed the annotation size limit of client-side apply.

## Usage

A `PaperlessInstance` describes one Paperless-NGX deployment: image, storage, and how it
reaches its database and cache. The default configuration manages both with
[CloudNativePG](https://cloudnative-pg.io) and a bundled Valkey, so install CloudNativePG
first:

    kubectl apply --server-side -f \
      https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.28/releases/cnpg-1.28.0.yaml

Then apply an instance, for example [`examples/paperlessinstance-minimal.yaml`](examples/paperlessinstance-minimal.yaml):

    kubectl apply -f examples/paperlessinstance-minimal.yaml

Until CloudNativePG is installed, the instance reports `Ready=False` with a reason naming
that as the blocker, and reconciles again automatically once it is. An instance can instead
point at an already-running database via `spec.database.external`.

By default, `kubectl delete` on a `PaperlessInstance` does not delete your documents: the four
PersistentVolumeClaims, the CloudNativePG database and the generated secrets all survive, and
a new instance created with the same name adopts them. Everything else — the Deployment,
Service, and the Valkey cache — is removed. Set `spec.deletionPolicy: Delete` to remove
everything instead, including your documents and database, the next time the instance is
deleted; switching the field back and forth on an existing instance takes effect on the next
reconcile, not just on creation.

## Releasing

Version bumps follow commit types: `feat` raises the minor version, `fix` the patch, and
everything else raises nothing. A repair to CI or tooling is therefore `ci` or `build`, not
`fix` — otherwise a release is cut whose entire content is changes users never run.


Merge the open release-please PR on `main`. That creates the `vX.Y.Z` tag and a GitHub
Release, and the same job then dispatches the `release` workflow for that tag, which builds
and pushes the image, chart, SBOM, signatures and provenance.

The dispatch is explicit rather than relying on the tag push, because a tag pushed with the
default `GITHUB_TOKEN` raises no events and would never start `release` on its own;
`workflow_dispatch` through the API is the documented exception. The workflow also still
accepts a manual run — `Actions` → `release` → `Run workflow`, or
`gh workflow run release.yaml -f tag=v0.2.0` — for re-publishing an existing tag.

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
