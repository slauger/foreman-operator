# Supply Chain Security

!!! note "Status"
    No images are published yet. The reusable workflows exist but no pipeline calls them. This page describes the planned controls and what is active today.

## Active today

| Control | Where |
|---|---|
| Dependency updates | Renovate, `renovate.json` |
| Go vulnerability scan | `govulncheck` in CI |
| Static analysis | `golangci-lint` with `gosec` in CI |
| Invisible character check | Unicode lint in CI |
| Generated code drift | `make check-manifests` in CI |
| Signed commits | GPG, by convention |

## Planned for images and charts

| Control | Tool | Workflow |
|---|---|---|
| Keyless image signing | [cosign](https://docs.sigstore.dev/cosign/) with GitHub Actions OIDC | `_container-build.yaml` |
| Build provenance | [SLSA](https://slsa.dev/) from BuildKit, `actions/attest-build-provenance` | `_container-build.yaml` |
| SBOM | SPDX from BuildKit, CycloneDX from [syft](https://github.com/anchore/syft) | `_container-build.yaml` |
| Policy validation | [Conforma](https://github.com/conforma) with `.conforma/policy.yaml` | `_conforma-validate.yaml` |
| Base image tracking | Renovate digests for UBI images | `renovate.json` |
| Containerfile linting | hadolint with `.hadolint.yaml` | To be added with the first Containerfile |

Once images are published, a signature can be verified like this:

```bash
cosign verify ghcr.io/slauger/foreman-operator:latest \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp 'github\.com/slauger/foreman-operator'
```

## Limits while the repository is private

- Keyless cosign signatures are recorded in the public Rekor transparency log. They expose image names and the repository identity. Run the build with `sign: false` until the project is published.
- GitHub artifact attestations for private repositories require GitHub Enterprise Cloud.
- GitHub Pages sites are public even for private repositories, so the documentation is not deployed automatically.

These statements come from the documentation of the respective services and have not been tested here.

## Policy exceptions

`.conforma/policy.yaml` allows only Red Hat base image registries. The planned `boot-files` image is based on CentOS Stream and contains only static files. It needs an explicit exception or a separate policy.

## License compliance

Images that ship GPL components are part of the supply chain obligations. Each published image carries an `org.opencontainers.image.licenses` label and the license texts, and has a source image with the matching SRPMs. See [Container Images](../concepts/images.md#licensing).
