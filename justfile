set shell := ["bash", "-euo", "pipefail", "-c"]

image := "ghcr.io/p3l1/paperless-ngx-operator"
tag := "dev"
cluster := "paperless-operator"
k3s_image := "rancher/k3s:v1.36.4-k3s1"
envtest_k8s := "1.37.0"
chart := "charts/paperless-ngx-operator"
cnpg_version := "1.28.0"

default:
    @just --list

# Verify required tools are present and install the commit hook.
setup:
    #!/usr/bin/env bash
    set -euo pipefail
    missing=0
    check() {
        if ! command -v "$1" >/dev/null 2>&1; then
            echo "missing: $1 — install with: $2" >&2
            missing=1
        fi
    }
    check go      "https://go.dev/dl/"
    check docker  "https://docs.docker.com/get-docker/"
    check helm    "brew install helm"
    check kubectl "brew install kubernetes-cli"
    check k3d     "brew install k3d"
    check kubeconform "brew install kubeconform"
    [ "$missing" -eq 0 ] || { echo "install the tools above, then re-run just setup" >&2; exit 1; }
    # syft and helm-docs are only needed by their own single recipe, so a warning rather than a failure.
    command -v syft >/dev/null 2>&1 || echo "note: syft absent; just sbom will not work (brew install syft)" >&2
    command -v helm-docs >/dev/null 2>&1 || echo "note: helm-docs absent; just docs will not work (brew install norwoodj/tap/helm-docs)" >&2
    install -m 0755 hack/commit-msg .git/hooks/commit-msg
    echo "all required tools present; commit-msg hook installed"

fmt:
    go fmt ./...

# Fails, without rewriting, on files go fmt would otherwise silently reformat.
fmt-check:
    #!/usr/bin/env bash
    set -euo pipefail
    unformatted=$(gofmt -l .)
    if [ -n "$unformatted" ]; then
        echo "not gofmt-ed (run: just fmt):" >&2
        echo "$unformatted" >&2
        exit 1
    fi

vet:
    go vet ./...

lint:
    go tool golangci-lint run ./...
    helm lint {{chart}}
    # kubeconform's default schema catalog has no schema for the CRD kind itself
    # (only for the custom resources it defines), so that kind is skipped here.
    helm template {{chart}} | kubeconform -strict -summary -skip CustomResourceDefinition -

# Fast tier: seconds, run on every change.
check: fmt-check vet lint
    go test ./api/... ./internal/... ./cmd/...

# Medium tier: envtest against a real API server, no cluster.
test:
    #!/usr/bin/env bash
    set -euo pipefail
    # --bin-dir must be absolute: go test runs the binary from the package dir, not
    # here, so a relative "-p path" result would no longer resolve at that point.
    export KUBEBUILDER_ASSETS="$(go tool setup-envtest use {{envtest_k8s}} --bin-dir {{justfile_directory()}}/.envtest -p path)"
    pkgs="./test/envtest/..."
    # internal/controller lands with the first reconciler; go test errors on a
    # package path that does not exist yet, so only add it once it does.
    if [ -d internal/controller ]; then
        pkgs="$pkgs ./internal/controller/..."
    fi
    go test $pkgs -count=1

build:
    go build -ldflags "-s -w -X github.com/p3l1/paperless-ngx-operator/internal/version.Version={{tag}} -X github.com/p3l1/paperless-ngx-operator/internal/version.Commit=$(git rev-parse --short HEAD)" -o bin/manager ./cmd

generate:
    #!/usr/bin/env bash
    set -euo pipefail
    # controller-gen needs api/ to exist; without CRD types there is nothing to generate.
    if [ ! -d api ]; then
        echo "no api/ directory yet; nothing to generate"
        exit 0
    fi
    go tool controller-gen object:headerFile=hack/boilerplate.go.txt paths=./api/...
    go tool controller-gen crd paths=./api/... output:crd:artifacts:config=config/crd/bases
    for f in config/crd/bases/*.yaml; do
        sh hack/crd-to-template.sh "$f" "{{chart}}/templates/crds/$(basename "$f")"
    done
    # internal/controller lands with the first reconciler (see `test`); until then
    # there are no RBAC markers, and the manager keeps only its static leases rule.
    if [ -d internal/controller ]; then
        go tool controller-gen rbac:roleName=manager-role paths=./internal/controller/... output:rbac:artifacts:config=config/rbac
    fi
    sh hack/rbac-to-chart.sh config/rbac/role.yaml {{chart}}/templates/rbac.yaml

# Fails when generated output is not committed, or the two chart versions drift.
verify: generate
    #!/usr/bin/env bash
    set -euo pipefail
    # --exit-code ignores untracked files, so it misses a CRD generated for the
    # first time; status --porcelain sees new and uncommitted files alike.
    changes=$(git status --porcelain -- config {{chart}} api)
    if [ -n "$changes" ]; then
        echo "generated output does not match what is committed:" >&2
        echo "$changes" >&2
        exit 1
    fi
    v=$(awk '/^version:/ {print $2; exit}' {{chart}}/Chart.yaml)
    a=$(awk '/^appVersion:/ {gsub(/"/, "", $2); print $2; exit}' {{chart}}/Chart.yaml)
    if [ "$v" != "$a" ]; then
        echo "chart version ($v) and appVersion ($a) differ" >&2
        exit 1
    fi
    echo "generated output is in sync; chart version $v"

cluster-up:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! k3d cluster list {{cluster}} >/dev/null 2>&1; then
        k3d cluster create {{cluster}} --image {{k3s_image}} --agents 0 --wait
    fi
    kubectl --context k3d-{{cluster}} cluster-info
    if ! kubectl --context k3d-{{cluster}} get crd clusters.postgresql.cnpg.io >/dev/null 2>&1; then
        kubectl --context k3d-{{cluster}} apply --server-side -f \
            "https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.28/releases/cnpg-{{cnpg_version}}.yaml"
        kubectl --context k3d-{{cluster}} -n cnpg-system wait --for=condition=Available \
            deployment/cnpg-controller-manager --timeout 3m
    fi

cluster-down:
    k3d cluster delete {{cluster}} || true

docker-build:
    docker build -t {{image}}:{{tag}} --build-arg VERSION={{tag}} --build-arg COMMIT=$(git rev-parse --short HEAD) .

deploy: docker-build cluster-up
    k3d image import {{image}}:{{tag}} -c {{cluster}}
    # The image tag never changes, so the pod template needs a per-deploy stamp:
    # otherwise the Deployment is byte-identical and Kubernetes rolls nothing.
    helm upgrade --install paperless-operator {{chart}} \
        --kube-context k3d-{{cluster}} \
        --namespace paperless-operator-system --create-namespace \
        --set image.repository={{image}} --set image.tag={{tag}} \
        --set image.pullPolicy=IfNotPresent \
        --set-string podAnnotations.deployedAt="$(date -u +%Y%m%dT%H%M%SZ)" \
        --wait --timeout 3m
    kubectl --context k3d-{{cluster}} -n paperless-operator-system \
        rollout status deployment/paperless-operator-paperless-ngx-operator --timeout 3m

# Full tier: minutes, run before a PR and in CI.
e2e: deploy
    go test ./test/e2e/... -count=1 -timeout 10m

sbom:
    syft scan {{image}}:{{tag}} -o spdx-json=operator.sbom.json
    syft scan dir:{{chart}} -o spdx-json=chart.sbom.json

docs:
    helm-docs --chart-search-root {{chart}}
