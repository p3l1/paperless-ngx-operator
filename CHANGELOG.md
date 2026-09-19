# Changelog

## [0.2.0](https://github.com/p3l1/paperless-ngx-operator/compare/paperless-ngx-operator-v0.1.0...paperless-ngx-operator-v0.2.0) (2026-09-19)


### Features

* add helm chart installing the operator ([706a2f4](https://github.com/p3l1/paperless-ngx-operator/commit/706a2f4dc16ad57407ec4aa8cdefc7b21fe70633))
* add manager entrypoint with health probes ([d30ae9d](https://github.com/p3l1/paperless-ngx-operator/commit/d30ae9d682a881641a1c8ffc1df5deff6007ed65))
* add PaperlessInstance CRD ([25c1251](https://github.com/p3l1/paperless-ngx-operator/commit/25c1251f0935aec216bd1213dea4a4a5d52136a7))
* **api:** add nil-safe accessors for CacheSpec and AdminSpec ([cb51740](https://github.com/p3l1/paperless-ngx-operator/commit/cb517402a318a320d81bb6c641301f7582c99ecf))
* **api:** add PaperlessInstance types ([bf86ef7](https://github.com/p3l1/paperless-ngx-operator/commit/bf86ef7fb02664da6c19bd79c54a9802eb161ca9))
* **api:** add spec.deletionPolicy, defaulting to Retain ([1432475](https://github.com/p3l1/paperless-ngx-operator/commit/14324752b1fecc26edc3a6feb50db01b7e5fd08b))
* **controller:** honor spec.deletionPolicy for stateful resources ([fc28848](https://github.com/p3l1/paperless-ngx-operator/commit/fc288480dd46611110248d2ebb120cd5bb323984))
* **controller:** reconcile PaperlessInstance ([e2aa1a8](https://github.com/p3l1/paperless-ngx-operator/commit/e2aa1a8fc69f8b4e9788da1b2fff651e5348c097))
* **resources:** add name helpers and secret builders ([7dac704](https://github.com/p3l1/paperless-ngx-operator/commit/7dac704cc24697778e8aabcf23391f39f39e3c42))
* **resources:** add readiness and liveness probes to the workload ([2275ab0](https://github.com/p3l1/paperless-ngx-operator/commit/2275ab0a41961e7f9ac09ad77fbb441208613a3a))
* **resources:** add storage, cache and database builders ([86ba920](https://github.com/p3l1/paperless-ngx-operator/commit/86ba920e42b43b9fd0a6f002e6b87f798d30ccef))
* **resources:** add workload builder ([1053817](https://github.com/p3l1/paperless-ngx-operator/commit/1053817c890fb22e0f1bea9ae4c79cd78bbc68f4))


### Bug Fixes

* add actions:read permission and extend security verification docs ([a852cb3](https://github.com/p3l1/paperless-ngx-operator/commit/a852cb322df14bebd06f05ecb10cde79615f0d27))
* address final review findings and pin all actions to SHAs ([e2b8030](https://github.com/p3l1/paperless-ngx-operator/commit/e2b8030eb2af930ab4c9380a4d0f77c9c638d356))
* **api:** give each storage volume its own kubectl explain description ([2090fd2](https://github.com/p3l1/paperless-ngx-operator/commit/2090fd286e89801cbd6ecbe8db1f719adeeb64f9))
* **api:** make status.conditions a mergeable list under Server-Side Apply ([1fe7fa5](https://github.com/p3l1/paperless-ngx-operator/commit/1fe7fa5278705191e54f6a38a315c511bde2bf44))
* **api:** materialise nested schema defaults through the structural chain ([af616a3](https://github.com/p3l1/paperless-ngx-operator/commit/af616a3956c49ab75b3d218783c42def33eecc64))
* **api:** pin the default image tag and stop inventing "latest" ([efcf1ac](https://github.com/p3l1/paperless-ngx-operator/commit/efcf1ac5c00bfbe56ab5e6567c496c38b373fbf9))
* **api:** reject setting both database.cnpg and database.external ([7fb0c03](https://github.com/p3l1/paperless-ngx-operator/commit/7fb0c032d574f54d73d2901f9a3734d98571fb45))
* **api:** require a cache url when the operator does not manage it ([5135b10](https://github.com/p3l1/paperless-ngx-operator/commit/5135b109a4f08ef0365cefd6ffc6a2b5b78ac96c))
* **api:** resolve volume size defaults in Go instead of typing Size as a pointer ([e456171](https://github.com/p3l1/paperless-ngx-operator/commit/e456171768c708285f86b1b3d1dc8700a4411615))
* **api:** stop using deprecated controller-runtime scheme.Builder ([0fb570d](https://github.com/p3l1/paperless-ngx-operator/commit/0fb570d16c11e42a1977c4bfcdd071bda719b9b7))
* **api:** validate image digest, external database port, and instance url ([ee433fc](https://github.com/p3l1/paperless-ngx-operator/commit/ee433fccef16d5b8ab3c711e4f3d12a44f4d6f31))
* **chart:** fail loudly if the CRD resource-policy annotation is not injected ([42d9ef5](https://github.com/p3l1/paperless-ngx-operator/commit/42d9ef58660ab89681794837be1cf3c8212893e3))
* **chart:** make RBAC generation correct and impossible to silently break ([a62440d](https://github.com/p3l1/paperless-ngx-operator/commit/a62440d3b651a7b319e1a582f1a9802e15500856))
* **chart:** rename crds.install to crds.enabled ([9dcd792](https://github.com/p3l1/paperless-ngx-operator/commit/9dcd792d810a0f17664f2ffa77f8a000c1b9c19e))
* **ci:** raise go toolchain to 1.26.1 and pin setup-go to latest patch ([5b408ff](https://github.com/p3l1/paperless-ngx-operator/commit/5b408ff717d944c53c1731fa5610fa1a487814fe))
* **controller:** address review findings on status, database and cache ([7585428](https://github.com/p3l1/paperless-ngx-operator/commit/7585428d979b47452610f8415f160fdf6d376e39))
* **controller:** requeue when CloudNativePG or a database secret is missing ([aa7e789](https://github.com/p3l1/paperless-ngx-operator/commit/aa7e789ed7de47b58cee6c4b003578ea933c0663))
* correct gh attestation verify command with oci scheme and repo flag ([3100d36](https://github.com/p3l1/paperless-ngx-operator/commit/3100d3685c1eb22b2fa1fcefbaa41461b0721bd9))
* **justfile:** drop project-phase comment and warn on missing helm-docs ([9c1eb32](https://github.com/p3l1/paperless-ngx-operator/commit/9c1eb326395b1111e1d569b5a0655db470cedac4))
* **justfile:** force a pod replacement on every deploy ([0f843e7](https://github.com/p3l1/paperless-ngx-operator/commit/0f843e7de75deeeeffc5034a79b2389710ff430b))
* **justfile:** make verify see untracked generated files ([9dc7953](https://github.com/p3l1/paperless-ngx-operator/commit/9dc795355a7352e55189a7e2a45d69532ead3f4b))
* **rbac:** drop list and watch on secrets from the manager ClusterRole ([7191be2](https://github.com/p3l1/paperless-ngx-operator/commit/7191be25506e390e6396902040aa2306fe2150ad))
* resolve envtest bin-dir to an absolute path in just test ([272945c](https://github.com/p3l1/paperless-ngx-operator/commit/272945c54dc81c922b4838e43249bd70f9590653))
* **resources:** send an allowed Host header from Paperless probes ([0ca3a07](https://github.com/p3l1/paperless-ngx-operator/commit/0ca3a07d0929e090b5dd84ca13435d8e95abad04))
* **resources:** use Recreate strategy for the Valkey Deployment ([8da5088](https://github.com/p3l1/paperless-ngx-operator/commit/8da5088d0961b0b3b6607c04172a965eae795210))
* **test:** make e2e assertions detect a genuinely unready operator ([7e0bea9](https://github.com/p3l1/paperless-ngx-operator/commit/7e0bea970e47d6279a465ded5d3fc83630c60cc3))
