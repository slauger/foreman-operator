# Platform Support

!!! note "Status"
    Target design. Behavior is stated from platform documentation and has not been qualified.

The operator detects OpenShift through the API groups `security.openshift.io` and `route.openshift.io`.

| Concern | k3s | OpenShift |
|---|---|---|
| Pod UID | Numeric UID from the image | Unset, assigned from the namespace range |
| Security policy | Pod Security `restricted` | `restricted-v2` SCC |
| Exposure with `Auto` | LoadBalancer | Route with passthrough |
| HTTP on port 80 | LoadBalancer | Plain Route or LoadBalancer |
| Load balancer | ServiceLB, or MetalLB for stable addresses | MetalLB Operator or platform integration |
| Multus | Manual install with k3s CNI paths | Built in |
| Provisioning privileges | Namespace exempt from `restricted` | Dedicated SCC |
| NetworkPolicy | Embedded controller, verify with the chosen CNI | OVN-Kubernetes |
| PVC for the proxy | `local-path`, node-bound | Depends on the CSI driver |
| cert-manager | Helm chart | cert-manager Operator for Red Hat OpenShift |
| PostgreSQL | CloudNativePG | CloudNativePG |
| Monitoring | Optional ServiceMonitor | User workload monitoring |

## Testing OpenShift behavior early

CI will run workloads under an arbitrary UID from the first controller on. Full qualification on OpenShift follows in the [roadmap](../roadmap.md).
