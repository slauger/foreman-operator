# Supply Chain Security

!!! note "Status"
    The `Images` workflow builds and pushes all images with `sign: false` while the repository is private. This page describes what is active today and the controls that are switched on once the repository is public.

## Active today

| Control | Where |
|---|---|
| Dependency updates | Renovate, `renovate.json` |
| Base image tracking | Renovate for the `FROM` lines of the Containerfiles |
| Go vulnerability scan | `govulncheck` in CI |
| Static analysis | `golangci-lint` with `gosec` in CI |
| Invisible character check | Unicode lint in CI |
| Generated code drift | `make check-manifests` in CI |
| Signed commits | GPG, by convention |
| Containerfile linting | hadolint with `.hadolint.yaml` in `_container-build.yaml` |
| Build provenance and SPDX SBOM | BuildKit, attached to every pushed image |
| SPDX and CycloneDX SBOMs | syft, uploaded as workflow artifacts |

## Switched on once the repository is public

| Control | Tool | Workflow |
|---|---|---|
| Keyless image signing | [cosign](https://docs.sigstore.dev/cosign/) with GitHub Actions OIDC | `_container-build.yaml` |
| GitHub build provenance | [SLSA](https://slsa.dev/) through `actions/attest-build-provenance` | `_container-build.yaml` |
| SBOM attestation | CycloneDX from [syft](https://github.com/anchore/syft), attested with cosign | `_container-build.yaml` |
| Policy validation | [Conforma](https://github.com/conforma) with `.conforma/policy.yaml` | `_conforma-validate.yaml`, not called by a pipeline yet |

Once images are signed, a signature can be verified like this:

```bash
cosign verify ghcr.io/slauger/foreman-operator:develop \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp 'github\.com/slauger/foreman-operator'
```

## Limits while the repository is private

- Keyless cosign signatures are recorded in the public Rekor transparency log. They expose image names and the repository identity. The `Images` workflow therefore runs with `sign: false` until the project is published.
- GitHub artifact attestations for private repositories require GitHub Enterprise Cloud.
- GitHub Pages sites are public even for private repositories, so the documentation is not deployed automatically.

These statements come from the documentation of the respective services and have not been tested here.

## Policy exceptions

`.conforma/policy.yaml` allows only Red Hat base image registries. The `boot-files` image copies static files from a CentOS Stream 9 build stage into a UBI 9 minimal image. Whether the policy accepts the build stage is not checked yet.

## License compliance

Images that ship GPL components are part of the supply chain obligations. Each image carries an `org.opencontainers.image.licenses` label. License texts and source images with the matching SRPMs are still missing. See [Container Images](../concepts/images.md#licensing).
