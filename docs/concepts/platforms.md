# Platform Support

!!! note "Status"
    The k3s column is tested on the E2E cluster for Foreman and its frontend. The OpenShift column is target design, stated from platform documentation, and has not been qualified.

The operator detects OpenShift through the `route.openshift.io/v1` `Route` kind at startup. On OpenShift it omits `fsGroup` from pod security contexts and resolves exposure `Auto` to a Route.

| Concern | k3s | OpenShift |
|---|---|---|
| Pod UID | Numeric UID from the image | Unset, assigned from the namespace range |
| Security policy | Pod Security `restricted` | `restricted-v2` SCC |
| Exposure with `Auto` | LoadBalancer | Route with passthrough |
| Gateway API | TLSRoute, tested with Envoy Gateway | Not checked |
| HTTP on port 80 | LoadBalancer, or HTTPRoute with `TLSRoute` | Not created by the operator yet; the Route has no insecure edge policy |
| Load balancer | ServiceLB, or MetalLB for stable addresses | MetalLB Operator or platform integration |
| Multus | Manual install with k3s CNI paths | Built in |
| Provisioning privileges | Namespace with Pod Security level `privileged` | Dedicated SCC, not written yet |
| NetworkPolicy | Embedded controller, verify with the chosen CNI | OVN-Kubernetes |
| PVC for the proxy | `local-path`, node-bound | Depends on the CSI driver |
| cert-manager | Helm chart | cert-manager Operator for Red Hat OpenShift |
| PostgreSQL | CloudNativePG | CloudNativePG |
| Monitoring | Metrics endpoint only, no ServiceMonitor yet | User workload monitoring |

## Testing OpenShift behavior early

The operator never sets a UID for the Foreman, frontend, and Smart Proxy containers, so the images must work under the UID the platform assigns. CI does not run under an arbitrary UID yet. Full qualification on OpenShift follows in the [roadmap](../roadmap.md).
