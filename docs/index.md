# Foreman Operator

A Kubernetes Operator that maps [Foreman](https://theforeman.org/) and Smart Proxy infrastructure onto native building blocks - CRDs, Secrets, cert-manager, CloudNativePG, and Multus - for running Foreman and bare-metal provisioning on **Kubernetes** and **OpenShift**.

!!! warning "Experimental"
    This is an unofficial, unsupported prototype. It is not affiliated with or endorsed by the Foreman project. The API will change without notice. Do not use it in production.

    Some concepts and ideas were adopted from [theforeman/foreman-kubernetes](https://github.com/theforeman/foreman-kubernetes). See [Acknowledgements](acknowledgements.md).

## Features

--8<-- "docs/_snippets/features.md"

## How It Works

| Kind | State | Purpose |
|---|---|---|
| **Foreman** | Implemented | FQDN, image, database and Valkey references; web, Dynflow, and migrations |
| **Frontend** | Implemented | Mutual TLS, exposure |
| **SmartProxy** | Implemented for `Provisioning` | One registered proxy with networks and features |
| **Release** | Planned | Compatible image sets, ordered upgrades |
| **Candlepin** | Planned | Subscription service for Katello |
| **Pulp** | Planned | Content service for Katello |

Start with [Getting Started](guides/getting-started.md) to install the operator, or with the [Architecture](concepts/architecture.md) for the design. The milestones are in the [Roadmap](roadmap.md).

## What is verified

On a k3s cluster with cert-manager, CloudNativePG, Multus, and Envoy Gateway, the Foreman migration, the web and Dynflow workloads, the Frontend through a Gateway API TLSRoute (`/api/v2/ping`), and the OAuth signature of the operator against a real Foreman are verified. The Smart Proxy pod, its registration, and DHCP, TFTP, and DNS are covered by the E2E test but have not been run on a cluster yet. See [Testing](development/testing.md).

## Scope

The first stage is the base deployment: Foreman with Smart Proxies for PXE, DHCP, TFTP, and DNS. BMC is open because `ipmitool` is not available in UBI 9. [Katello](concepts/katello.md) is a later stage.

## License

Apache License 2.0. Foreman and Smart Proxy are separate projects licensed under GPL-3.0.
