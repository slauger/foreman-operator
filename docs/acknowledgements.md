# Acknowledgements

This project is an independent experiment. It is not affiliated with or endorsed by the Foreman project.

## foreman-kubernetes

[theforeman/foreman-kubernetes](https://github.com/theforeman/foreman-kubernetes) is the community effort to run Foreman with Katello on Kubernetes. Several concepts and ideas here were adopted from that work and its [RFC](https://community.theforeman.org/t/rfc-first-class-kubernetes-deployment-for-foreman-katello-candlepin-and-pulp/47739), including:

- running migrations as Jobs before application rollouts
- splitting web and task workloads with independent replica counts
- recurring tasks as CronJobs
- separate execution proxies
- pinned compatibility sets for images

The two projects differ in approach:

| | foreman-kubernetes | foreman-operator |
|---|---|---|
| Deployment | Helm charts with an optional release controller | Go operator with composable CRDs |
| First scope | Foreman with Katello | Foreman with Smart Proxy for provisioning |
| Images | Published upstream images | Own UBI 9 images |

No code was copied from that repository.

## foremanctl

[theforeman/foremanctl](https://github.com/theforeman/foremanctl) documents how Foreman runs in containers today. The process split, commands, and required settings were derived from it. No code or templates were copied.

## openvox-operator

The repository layout and operator patterns follow [slauger/openvox-operator](https://github.com/slauger/openvox-operator).

## Licenses

This repository is licensed under Apache-2.0. Foreman, Smart Proxy, `foremanctl`, and `foreman-kubernetes` are licensed under GPL-3.0.
