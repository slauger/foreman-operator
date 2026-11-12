# Releasing

Releases are cut from `main` with the `Release` workflow, which is started by hand. Changes reach `main` through a pull request from `develop`.

## Steps

1. Open a pull request from `develop` to `main` and merge it once CI is green.
2. Start the workflow on `main`:

    ```bash
    gh workflow run release.yaml --ref main
    ```

3. Check the release on GitHub: notes, chart packages, and SBOMs.

## Version

[semantic-release](https://semantic-release.gitbook.io/) derives the version from the [Conventional Commits](https://www.conventionalcommits.org/) on `main` since the last tag, see `.releaserc.json`:

| Commit | Release |
|---|---|
| `fix:` | Patch |
| `feat:` | Minor |
| Breaking change (`feat!:`, `fix!:`) | Minor, while the API is `v1alpha1` |

Without such commits, the workflow publishes nothing. If the repository has no `v*` tag yet, the workflow first tags the initial commit as `v0.0.0`, so the first release is `v0.1.0`.

## Published artifacts

| Artifact | Location | Version |
|---|---|---|
| Operator image | `ghcr.io/slauger/foreman-operator` | `<version>` and `latest` |
| Component images | `ghcr.io/slauger/foreman-operator-{foreman,smart-proxy,boot-files,frontend}` | `<version>` and `latest` |
| `foreman-operator` chart | `oci://ghcr.io/slauger/charts/foreman-operator` | `<version>` |
| `foreman-db-cnpg` chart | `oci://ghcr.io/slauger/charts/foreman-db-cnpg` | `<version>` |
| Chart packages, SPDX and CycloneDX SBOMs | GitHub release assets | `v<version>` |

The workflow writes the version into `version` and `appVersion` of both charts. The `foreman-operator` chart uses its `appVersion` for the operator image and, through `--default-image-tag`, for all component images, so one chart version always runs the images built from the same commit.

The release notes list the images and name where the sources of the shipped packages are published, see [Container Images](../concepts/images.md#licensing).

## Signing

The workflow checks the repository visibility. On a public repository, it signs the images and charts with keyless cosign and attaches the SBOM and build provenance attestations. On a private repository, it skips these steps, see [Supply Chain Security](../guides/supply-chain-security.md).

## Package visibility

New ghcr.io packages are private. After the first release, set the visibility of each image package and of `charts/foreman-operator` and `charts/foreman-db-cnpg` to public in the package settings.
