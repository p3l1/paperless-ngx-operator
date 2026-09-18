# ADLC profile

## verify.commands

- test: `just test`
- lint: `just lint`
- format: `just fmt`
- build: `just build`
- typecheck: `just vet`
- e2e: `just e2e`
- verify-generated: `just verify`

## pm

- system: GitHub Issues, in this repository
- labels: `feature`, `bug`, `chore`
- A slice is an issue; its tasks are sub-issues.

## git

- Default branch: `main`
- Branch naming: `<type>/<issue-number>-<slug>`, e.g. `feat/12-paperless-instance`
- Commits: Conventional Commits, GPG-signed
- PRs: squash merge; the PR title becomes the commit message and is linted
