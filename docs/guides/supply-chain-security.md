# Supply Chain Security

!!! note "Status"
    Release images and charts are signed with keyless cosign and carry SBOM and build provenance attestations. Images built from `develop` and feature branches are not signed yet.

## Controls

| Control | Where |
|---|---|
| Dependency updates | Renovate, `renovate.json` |
| Base image tracking | Renovate for the `FROM` lines of the Containerfiles |
| Foreman and plugin versions | Pinned RPMs, tracked by Renovate with the `rpm` datasource, see [Container Images](../concepts/images.md#versions) |
| Base package errata | Weekly rebuild of `develop` |
| Go vulnerability scan | `govulncheck` in CI |
| Static analysis | `golangci-lint` with `gosec` in CI |
| Invisible character check | Unicode lint in CI |
| Generated code drift | `make check-manifests` in CI |
| Signed commits | GPG, by convention |
| Containerfile linting | hadolint with `.hadolint.yaml` in `_container-build.yaml` |
| Build provenance and SPDX SBOM | BuildKit, attached to every pushed image |
| SPDX and CycloneDX SBOMs | syft; workflow artifacts, and release assets for releases |

## Release signing

The `Release` workflow signs only when the repository is public, which it is:

| Control | Tool | Workflow |
|---|---|---|
| Keyless image and chart signing | [cosign](https://docs.sigstore.dev/cosign/) with GitHub Actions OIDC | `_container-build.yaml`, `release.yaml` |
| GitHub build provenance | [SLSA](https://slsa.dev/) through `actions/attest-build-provenance` | `_container-build.yaml` |
| SBOM attestation | CycloneDX from [syft](https://github.com/anchore/syft), attested with cosign | `_container-build.yaml` |
| Policy validation | [Conforma](https://github.com/conforma) with `.conforma/policy.yaml` | `_conforma-validate.yaml`, not called by a pipeline yet |

Verify an image signature:

```bash
cosign verify ghcr.io/slauger/foreman-operator:0.1.0 \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp 'github\.com/slauger/foreman-operator'
```

Verify a chart the same way with `ghcr.io/slauger/charts/foreman-operator:0.1.0`, and the build provenance with `gh attestation verify oci://ghcr.io/slauger/foreman-operator:0.1.0 --owner slauger`.

The `Images` workflow for `develop` and feature branches still runs with `sign: false`, which dates from the time the repository was private. Use release images where signatures matter.

## Policy exceptions

`.conforma/policy.yaml` allows only Red Hat base image registries. The `foreman`, `smart-proxy`, and `boot-files` images copy packages or files from CentOS Stream 9 build stages, whose signatures are checked during the build. Whether the policy accepts these build stages is not checked yet.

## License compliance

Images that ship GPL components are part of the supply chain obligations. Each image carries an `org.opencontainers.image.licenses` label. Release SBOMs list every package with its source RPM, and the release notes point to the published sources. See [Container Images](../concepts/images.md#licensing).
