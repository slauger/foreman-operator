# Migration

!!! note "Status"
    A plan only. The Foreman 5.0 release notes state that there is no supported migration from package installations yet.

1. Upgrade the package installation to a release that the target supports.
2. Translate answer files with `foremanctl migrate`, and treat unmapped parameters as a manual review list.
3. Rehearse on a restored copy in an isolated network.
4. Import the CA into a Secret for a cert-manager CA issuer, with unencrypted keys, and export the other secrets.
5. Restore the database into a CloudNativePG cluster.
6. Move DHCP data to the new proxies, one network at a time.
7. Deploy with the existing FQDN, then cut over DNS with writers stopped.
8. Validate with a test PXE build and sample hosts.

Katello adds the Candlepin and Pulp databases and copying `/var/lib/pulp`. Moving that content to S3 is the riskiest step.
