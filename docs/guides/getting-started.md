# Getting Started

This guide installs a released operator from the chart registry and creates a Foreman with its frontend and, optionally, a provisioning Smart Proxy. The resources follow `config/samples` and the E2E test in `tests/e2e/mvp/resources.yaml`.

!!! warning "Experimental"
    This is an unofficial, unsupported prototype. It is not affiliated with or endorsed by the Foreman project. Use a test cluster.

## Prerequisites

| Component | Required | Used for |
|---|---|---|
| Kubernetes 1.30 or later | Yes | Tested on k3s |
| [cert-manager](https://cert-manager.io/) | Yes, for `issuerRef` | Internal CA, server, client, and proxy certificates |
| [CloudNativePG](https://cloudnative-pg.io/) | Yes | PostgreSQL for Foreman |
| Valkey | Yes | Rails cache and Dynflow. The operator does not deploy it |
| Gateway API with TLSRoute | No | Exposure `TLSRoute`. Tested with Envoy Gateway |
| A LoadBalancer implementation | No | Exposure `LoadBalancer` or `Auto` outside OpenShift |
| [Multus](https://github.com/k8snetworkplumbingwg/multus-cni) | For provisioning | Attaches the Smart Proxy to the provisioning network |

The operator detects cert-manager, the Gateway API, and OpenShift Routes when it starts. Install them before the operator, or restart the operator afterwards.

The images are amd64 only, except the operator and the frontend.

## Install the operator

Releases publish the charts to `oci://ghcr.io/slauger/charts`, see the [releases](https://github.com/slauger/foreman-operator/releases) for the versions. Helm installs CRDs only on the first install and never upgrades them, so apply them explicitly from the chart before every install or upgrade:

```bash
VERSION=0.1.0

helm pull oci://ghcr.io/slauger/charts/foreman-operator --version "${VERSION}" --untar
kubectl apply --server-side -f foreman-operator/crds/

helm install foreman-operator oci://ghcr.io/slauger/charts/foreman-operator \
  --version "${VERSION}" \
  --namespace foreman-operator \
  --create-namespace
```

The chart `appVersion` is the tag of the operator image and the default tag of the Foreman, frontend, Smart Proxy, and boot-files images. By default, the operator watches all namespaces. See the [chart README](https://github.com/slauger/foreman-operator/blob/main/charts/foreman-operator/README.md) for all values. To test the `develop` branch instead, install the chart from a checkout as described there.

## Create the namespace and database

```bash
kubectl create namespace foreman

helm install foreman-db oci://ghcr.io/slauger/charts/foreman-db-cnpg \
  --version "${VERSION}" \
  --namespace foreman
```

The chart creates the CloudNativePG `Cluster` `foreman-db`. CloudNativePG then writes the Secret `foreman-db-app`, which the `Foreman` reads.

## Create the internal CA

Foreman, the frontend client verification, and Smart Proxies share one internal CA. See [Certificates](../concepts/certificates.md) for the reasoning.

```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: selfsigned
  namespace: foreman
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: foreman-internal-ca
  namespace: foreman
spec:
  isCA: true
  commonName: foreman-internal-ca
  secretName: foreman-internal-ca
  duration: 175200h
  privateKey:
    algorithm: RSA
    size: 4096
    rotationPolicy: Never
  issuerRef:
    name: selfsigned
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: foreman-internal-ca
  namespace: foreman
spec:
  ca:
    secretName: foreman-internal-ca
```

## Provide Valkey

Any Valkey or Redis-compatible server without authentication works. For a test, a single Deployment is enough:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: valkey
  namespace: foreman
spec:
  selector:
    app: valkey
  ports:
    - port: 6379
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: valkey
  namespace: foreman
spec:
  replicas: 1
  selector:
    matchLabels:
      app: valkey
  template:
    metadata:
      labels:
        app: valkey
    spec:
      containers:
        - name: valkey
          image: quay.io/sclorg/valkey-8-c10s:latest
          ports:
            - containerPort: 6379
```

## Create Foreman

Replace `foreman.example.com` with a name that resolves to the address the frontend is exposed on. The frontend runs as the container `httpd` in every web pod, so `web.replicas` also sets the number of frontends.

```yaml
apiVersion: platform.theforeman.org/v1alpha1
kind: Foreman
metadata:
  name: foreman
  namespace: foreman
spec:
  fqdn: foreman.example.com
  tls:
    issuerRef:
      name: foreman-internal-ca
  database:
    cluster:
      name: foreman-db
  valkey:
    host: valkey
  web:
    replicas: 1
  frontend:
    tls:
      server:
        issuerRef:
          name: foreman-internal-ca
      clientCA:
        issuerRef:
          name: foreman-internal-ca
    exposure:
      type: Auto
```

`Auto` makes the Service `foreman-web` a LoadBalancer, or creates a passthrough Route on OpenShift. For a Gateway with TLS passthrough, use:

```yaml
  frontend:
    exposure:
      type: TLSRoute
      gatewayRef:
        name: foreman
        tlsSectionName: https
        httpSectionName: http
```

For a server certificate from a public issuer, set `frontend.tls.server.issuerRef` to that issuer. Keep `frontend.tls.clientCA` on the internal CA.

## Watch the rollout

```bash
kubectl -n foreman get foremen
kubectl -n foreman get jobs,deployments,pods
```

The `Foreman` passes these phases:

1. `Pending` until the database Secret exists.
2. `Migrating` while the Job `foreman-migrate-<hash>` runs `db:migrate`, `db:seed`, and `upgrade:run`. The first migration takes several minutes.
3. `Progressing` while web and Dynflow roll out.
4. `Running` when web and the Dynflow orchestrator are available.

If the migration Job fails, the phase is `Failed`. Read the Job logs, fix the cause, and delete the Job to retry.

## Log in

The operator generates the admin password into the Secret `<name>-secrets`. The user name is `admin`.

```bash
kubectl -n foreman get secret foreman-secrets -o jsonpath='{.data.admin-password}' | base64 -d; echo
```

Keep a copy of the whole Secret. It also holds the encryption key of the database, and it is not deleted together with the `Foreman`.

With the server certificate from the internal CA, clients must trust that CA:

```bash
kubectl -n foreman get secret foreman-internal-ca -o jsonpath='{.data.ca\.crt}' | base64 -d > foreman-ca.crt
curl --cacert foreman-ca.crt https://foreman.example.com/api/v2/ping
```

## Add a provisioning Smart Proxy

This step is covered by the E2E test but has not been run on a cluster yet. It needs Multus and a NetworkAttachmentDefinition for the provisioning network.

The dnsmasq container runs as root with `NET_ADMIN` and other capabilities, so the namespace needs the Pod Security level `privileged`:

```bash
kubectl label namespace foreman pod-security.kubernetes.io/enforce=privileged --overwrite
```

The Smart Proxy assigns its static address itself, so the NetworkAttachmentDefinition needs no IPAM:

```yaml
apiVersion: k8s.cni.cncf.io/v1
kind: NetworkAttachmentDefinition
metadata:
  name: pxe
  namespace: foreman
spec:
  config: |
    {
      "cniVersion": "1.0.0",
      "name": "pxe",
      "type": "bridge",
      "bridge": "pxe0",
      "ipam": {}
    }
```

For DNS, store a TSIG key in a Secret under `key`:

```bash
kubectl -n foreman create secret generic tsig --from-file=key=tsig.key
```

```yaml
apiVersion: platform.theforeman.org/v1alpha1
kind: SmartProxy
metadata:
  name: pxe
  namespace: foreman
spec:
  foremanRef: foreman
  profile: Provisioning
  url: https://pxe.foreman.svc:8443
  tls:
    issuerRef:
      name: foreman-internal-ca
  networks:
    - attachment: pxe
      address: 10.99.0.10/24
  features:
    tftp: {}
    templates:
      url: http://10.99.0.10:8000
    dhcp:
      provider: dnsmasq
      subnets:
        - network: 10.99.0.0/24
          range: 10.99.0.100-10.99.0.150
          gateway: 10.99.0.1
    dns:
      server: dns.example.com
      keySecretRef: tsig
  storage:
    size: 5Gi
```

Once the proxy pod is available and the `Foreman` is `Ready`, the operator registers the proxy in Foreman under the name `pxe` and sets `status.foremanID`. Deleting the `SmartProxy` removes it from Foreman. The PVC `pxe-data` stays.

## Next steps

- [API Reference](../reference/index.md) for all fields
- [Provisioning](../concepts/provisioning.md) for the pod layout and network rules
- [Upgrades](../concepts/upgrades.md) for image changes and migrations
