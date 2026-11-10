# Foreman Operator

A Kubernetes Operator that maps [Foreman](https://theforeman.org/) and Smart Proxy infrastructure onto native building blocks - CRDs, Secrets, cert-manager, CloudNativePG, and Multus - for running Foreman and bare-metal provisioning on **Kubernetes** and **OpenShift**.

!!! warning "Experimental"
    This is an unofficial, unsupported prototype. It is not affiliated with or endorsed by the Foreman project. No controllers exist yet, and the API will change without notice. Do not use it in production.

    Some concepts and ideas were adopted from [theforeman/foreman-kubernetes](https://github.com/theforeman/foreman-kubernetes). See [Acknowledgements](acknowledgements.md).

## Planned Features

--8<-- "docs/_snippets/features.md"

## How It Works

| Kind | State | Purpose |
|---|---|---|
| **Foreman** | API defined | FQDN, image, plugins, database and Valkey references |
| **Frontend** | API defined | Mutual TLS, path routing, exposure |
| **SmartProxy** | API defined | One registered proxy with a profile, networks, and features |
| **Release** | Planned | Compatible image sets, ordered upgrades |
| **Candlepin** | Planned | Subscription service for Katello |
| **Pulp** | Planned | Content service for Katello |

Start with the [Architecture](concepts/architecture.md). The first milestones are in the [Roadmap](roadmap.md).

## Scope

The first stage is the base deployment: Foreman with Smart Proxies for PXE, DHCP, TFTP, DNS, and BMC. [Katello](concepts/katello.md) is a later stage.

## License

Apache License 2.0. Foreman and Smart Proxy are separate projects licensed under GPL-3.0.
