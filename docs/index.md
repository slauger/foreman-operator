# Foreman Operator

A Kubernetes Operator that maps [Foreman](https://theforeman.org/) and Smart Proxy infrastructure onto native building blocks - CRDs, Secrets, cert-manager, CloudNativePG, and Multus - for running Foreman and bare-metal provisioning on **Kubernetes**.

!!! warning "Experimental"
    This is an unofficial, unsupported prototype. It is not affiliated with or endorsed by the Foreman project. The API will change without notice. Do not use it in production.

    Some concepts and ideas were adopted from [theforeman/foreman-kubernetes](https://github.com/theforeman/foreman-kubernetes). See [Acknowledgements](acknowledgements.md).

## Features

--8<-- "docs/_snippets/features.md"

## How It Works

| Kind | State | Purpose |
|---|---|---|
| **Foreman** | Implemented | FQDN, image, plugins, database and Valkey references; web with the frontend sidecar, Dynflow, recurring tasks, and checked migrations |
| **SmartProxy** | Implemented for `Provisioning` and `Execution` | One registered proxy with networks and features |
| **Release** | Planned | Compatible image sets |
| **Candlepin** | Planned | Subscription service for Katello |
| **Pulp** | Planned | Content service for Katello |

Start with [Getting Started](guides/getting-started.md) to install the operator, or with the [Architecture](concepts/architecture.md) for the design. The milestones are in the [Roadmap](roadmap.md).

## What is verified

The tests run on a single-node k3s cluster with cert-manager, CloudNativePG, Multus, and Envoy Gateway, see [Testing](development/testing.md).

| Area | Verified |
|---|---|
| Foreman | Migration from an empty database, web and Dynflow, recurring task CronJobs, the frontend sidecar through a Gateway API TLSRoute with Puma reachable only over its Unix socket |
| Plugins | Only the enabled plugins load; a boot ISO from `foreman_bootdisk`; the KubeVirt, Proxmox, and VMware compute resource providers |
| Provisioning proxy | Registration with the features BMC, DHCP, DNS, Discovery, Logs, TFTP, and Templates; a DHCP reservation that reaches a client on a fake VLAN and survives a new pod; a TFTP download of the boot loader; a DNS record in BIND through nsupdate; a discovered host that reaches Foreman through the proxy; unregistration on delete |
| Execution proxy | A Remote Execution job and an Ansible job on a target over SSH |
| Upgrades | From the released `0.1.0` chart to the current images, with data and proxy registrations kept and the smoke checks passing |
| Restore | Database, secrets, and proxy registrations restored in the documented order, with encrypted columns readable |

Not tested: a real VLAN, a real NIC, real PXE firmware, and a complete host build.

## Scope

The first stage is the base deployment: Foreman with Smart Proxies for provisioning and Remote Execution. [Katello](concepts/katello.md) is a later stage.

## License

Apache License 2.0. Foreman and Smart Proxy are separate projects licensed under GPL-3.0.
