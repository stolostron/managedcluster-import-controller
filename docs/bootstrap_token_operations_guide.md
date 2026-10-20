# Bootstrap Token and Client Certificate Operations Guide

## Overview

When you import a managed cluster into Red Hat Advanced Cluster Management (RHACM), two types of credentials are used for secure communication between the hub cluster and managed clusters:

1. **Bootstrap Token** - 360-day token for initial authentication (used during import/re-bootstrapping)
2. **Client Certificates** - 30-day certificates for ongoing communication (auto-rotate before expiration)

Both credentials renew automatically. This guide explains when they expire, how renewal works, and what you need to monitor.

---

## Bootstrap Token

### What It Is

A ServiceAccount JWT token that allows the klusterlet (agent) on the managed cluster to initially authenticate with the hub and request long-term client certificates.

### Lifecycle

| Property | Value |
|----------|-------|
| **Lifetime** | 360 days (1 year) |
| **Renewal Threshold** | 72 days before expiration (20% of lifetime remaining) |
| **Check Frequency** | Approximately every 10 minutes |
| **Used When** | Initial cluster import or when client certificates need to be reissued |

### How Renewal Works

**Automatic Process (No User Action Required):**

1. **Hub-side renewal:**
   - Approximately every 10 minutes, the hub controller checks all bootstrap tokens
   - When a token has less than 72 days remaining, a new token is automatically generated
   - The new token is packaged and delivered to the managed cluster via ManifestWork

2. **Managed cluster receives update:**
   - The work agent on the managed cluster detects the updated bootstrap token
   - Applies the new token to the local secret (`bootstrap-hub-kubeconfig` in `open-cluster-management-agent` namespace)
   - Klusterlet uses the new token if/when it needs to re-bootstrap

### When Bootstrap Token Is Used

The bootstrap token is only actively used in these scenarios:
- **Initial cluster import** - During first-time setup when no client certificates exist yet
- **Re-bootstrapping** - When existing client certificates have expired or been deleted and need to be reissued
- **Disaster recovery** - After credential loss requiring fresh authentication

**Important:** During normal certificate renewal (before expiration), the bootstrap token is NOT used. The registration agent uses the current valid client certificates to authenticate CSR requests for new certificates.

During normal operations, the bootstrap token sits idle in the secret and is not actively used.

---

## Client Certificates

### What They Are

X.509 certificates issued by the hub cluster that the klusterlet uses for all ongoing API communication with the hub. These certificates are automatically rotated before expiration.

### Lifecycle

| Property | Value |
|----------|-------|
| **Lifetime** | 30 days (OpenShift default) |
| **Renewal Threshold** | ~20% of lifetime remaining (~6 days before expiration) |
| **Check Frequency** | Continuously monitored by registration agent |
| **Used When** | All ongoing communication with hub |

**Note:** The certificate lifetime is controlled by the hub cluster's CSR signing controller configuration (`--cluster-signing-duration` flag). The OpenShift default is 30 days (`720h`), and `managedcluster-import-controller` always runs on OpenShift clusters.

### How Renewal Works

**Automatic Process (No User Action Required):**

1. **Managed cluster initiates:**
   - The registration agent on the managed cluster continuously monitors certificate expiration
   - When certificates approach expiration threshold, the agent creates a CertificateSigningRequest (CSR)
   - **Important:** The agent uses its **current valid client certificates** to authenticate this CSR request (NOT the bootstrap token)

2. **Hub approves:**
   - The hub's CSR controller automatically approves the CSR
   - New certificates are issued and made available to the managed cluster

3. **Managed cluster updates:**
   - The registration agent retrieves the new certificates
   - Updates the local secret (`hub-kubeconfig-secret` in `open-cluster-management-agent` namespace)
   - Begins using new certificates for API communication

This process happens transparently while the klusterlet continues normal operations. The bootstrap token is **not used** during normal certificate renewal—it's only needed during initial import or when re-bootstrapping after certificate expiration.

---

## Operations Monitoring

### Verify Your Configuration

Before setting up monitoring, verify your hub cluster's certificate signing configuration:

```bash
# Check certificate signing duration (OpenShift)
kubectl get pod -n openshift-kube-controller-manager -l app=kube-controller-manager -o yaml | \
  grep -o "cluster-signing-duration=[0-9]*h" | \
  awk -F'[=h]' '{print "Certificate lifetime: " $2/24 " days (" $2 " hours)"}'

# For vanilla Kubernetes
kubectl get pod -n kube-system -l component=kube-controller-manager -o yaml | \
  grep -o "cluster-signing-duration=[0-9]*h" | \
  awk -F'[=h]' '{print "Certificate lifetime: " $2/24 " days (" $2 " hours)"}'
```

**Common values:**
- `720h` = 30 days (OpenShift default)
- `8760h` = 365 days (1 year)

Use this value to set appropriate certificate expiration alerts below.

---

### What to Monitor

#### 1. Cluster Availability
**Metric:** ManagedCluster `ManagedClusterConditionAvailable` status condition

**Normal State:**
```
Status: True
Reason: ManagedClusterAvailable
Message: Managed cluster is available
```

**Alert:** If status becomes `False` or `Unknown` for more than 10 minutes

#### 2. Import Secret Status
**Check:** Hub cluster namespace per managed cluster

```bash
# Check if import secret exists
kubectl get secret -n <cluster-name> <cluster-name>-import

# Verify token expiration is not approaching
kubectl get secret -n <cluster-name> <cluster-name>-import \
  -o jsonpath='{.data.expiration}' | base64 -d
```

**Normal:** Token expiration should be >72 days in the future

#### 3. Client Certificate Expiration
**Check:** Managed cluster `open-cluster-management-agent` namespace

```bash
# On managed cluster
kubectl get secret -n open-cluster-management-agent hub-kubeconfig-secret \
  -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -enddate
```

**Normal:** Certificate should have >6 days before expiration (typically renews around 6 days before expiry)

#### 4. Controller Health
**Check:** Hub cluster `multicluster-engine` namespace

```bash
# Verify import controller is running
kubectl get deployment -n multicluster-engine \
  managedcluster-import-controller-v2
```

**Normal:** All pods should be Ready

---

## Failure Scenarios and Recovery

### Scenario 1: Bootstrap Token Expired

**Symptoms:**
- Managed cluster available status is normal (uses client certificates)
- If client certificates also expire, cluster becomes unavailable
- Hub controller logs show token expiration errors

**Auto-Recovery:**
If the import controller pod is running, the token renews automatically, typically within 10-15 minutes.

**Manual Recovery:**

First, check the token's current expiration:
```bash
# Check token expiration on hub
kubectl get secret -n <cluster-name> <cluster-name>-import \
  -o jsonpath='{.data.expiration}' | base64 -d
```

If the expiration date is in the past, trigger immediate renewal:
```bash
# Trigger immediate token renewal by adding a temporary label
kubectl label managedcluster <cluster-name> token-renewal=true

# Remove label after renewal completes
kubectl label managedcluster <cluster-name> token-renewal-
```

### Scenario 2: Client Certificates Expired

**Symptoms:**
- Managed cluster available status becomes `Unknown`
- Hub controller shows authentication errors
- Klusterlet logs show certificate errors

**Auto-Recovery:**
1. Klusterlet automatically attempts to re-bootstrap using the **bootstrap token** from the `bootstrap-hub-kubeconfig` secret. The klusterlet doesn't continuously monitor for expiration - it relies on Kubernetes liveness probes (every 10 seconds) to detect expired certificates and restart the pod, which then triggers automatic re-bootstrap on startup.
2. Requests new client certificates via CSR
3. Hub CSR approval controller (in `managedcluster-import-controller`) auto-approves and kube-controller-manager issues new certificates
4. Agent writes new certificates to `hub-kubeconfig-secret` and switches to normal operation mode
5. Cluster reconnects within minutes

**Note:** This recovery timeline depends on the CSR approval controller running in `managedcluster-import-controller`. If the controller is unavailable, CSR approval will not occur automatically and manual intervention will be required. This is different from normal certificate renewal, where the agent uses existing valid certificates to request new ones. Here, because the certificates expired, the agent must fall back to using the bootstrap token to re-establish credentials.

**Manual Recovery (if bootstrap token also expired):**

**Important:** When client certificates are expired, the work agent cannot authenticate to the hub to fetch updated ManifestWorks. You must manually extract and apply the new bootstrap token directly to the spoke cluster.

```bash
# 1. On hub: Trigger bootstrap token renewal
kubectl label managedcluster <cluster-name> token-renewal=true

# 2. Wait 10-15 minutes for token regeneration, then extract import.yaml
IMPORT_FILE=$(mktemp -t import-XXXXXX.yaml)
chmod 600 "$IMPORT_FILE"
kubectl get secret -n <cluster-name> <cluster-name>-import \
  -o jsonpath='{.data.import\.yaml}' | base64 -d > "$IMPORT_FILE"

# 3. Apply import.yaml to the spoke cluster
kubectl apply -f "$IMPORT_FILE"

# 4. Verify the bootstrap secret was updated on spoke
kubectl get secret bootstrap-hub-kubeconfig -n open-cluster-management-agent \
  -o jsonpath='{.metadata.creationTimestamp}'

# 5. Monitor klusterlet logs for automatic re-bootstrap
kubectl logs -n open-cluster-management-agent -l app=klusterlet-registration-agent -f

# Optional: Force immediate restart (not required - the agent watches the secret)
# kubectl delete pod -n open-cluster-management-agent -l app=klusterlet

# 6. Clean up credential file
rm -f "$IMPORT_FILE"
```

**Security Notes:**
- Use `mktemp` to create unpredictable temporary file paths
- Set restrictive permissions (mode 0600) to protect credentials
- Remove credential files immediately after use

**Note:** Applying `import.yaml` on the spoke is safe and idempotent - it only updates klusterlet components and credentials, and does not affect any applications deployed via ManifestWork.

**Warning:** Do not delete and re-import the ManagedCluster on the hub in production, as this will remove all ACM-deployed applications and configurations from the spoke cluster.

### Scenario 3: Import Controller Unavailable

**Symptoms:**
- Import controller pods not running
- Bootstrap token renewal does not occur

**Impact:**
- **Bootstrap token renewal:** Will not occur while controller is unavailable
- **Client certificate renewal:** Cannot complete while controller is unavailable, because CSR auto-approval runs in the import controller
- **Existing certificates:** Continue working until their recorded expiration

**Critical:** Do not rely on fixed day counts. Certificates may expire sooner depending on when they were last issued. Check actual expiration dates and restore the controller before any certificate expires.

**To assess impact:**
```bash
# Check bootstrap token expiration on hub
kubectl get secret -n <cluster-name> <cluster-name>-import \
  -o jsonpath='{.data.expiration}' | base64 -d

# Check client certificate expiration on each managed cluster
kubectl get secret -n open-cluster-management-agent hub-kubeconfig-secret \
  -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -enddate
```

**Recovery:**
```bash
# 1. Restore import controller
kubectl rollout restart deployment -n multicluster-engine \
  managedcluster-import-controller-v2

# 2. Verify controller is running
kubectl get pods -n multicluster-engine -l app=managedcluster-import-controller

# 3. Wait 10-15 minutes for automatic token renewal

# 4. Verify tokens renewed
kubectl get secret -n <cluster-name> <cluster-name>-import \
  -o jsonpath='{.data.expiration}' | base64 -d
```

### Scenario 4: Network Partition

**Symptoms:**
- Managed cluster cannot reach hub
- Work agent cannot receive updates

**Auto-Recovery:**
Once network connectivity is restored:
1. Work agent automatically syncs pending ManifestWork updates
2. Bootstrap token renewal (if pending) is applied
3. Client certificate rotation (if needed) proceeds automatically

**No manual intervention required** - system is designed to handle temporary network issues.

---

## Key Operational Points

### ✅ Normal Operations

**No action required for:**
- Bootstrap token renewal (automatic when reaching renewal threshold - 72 days before expiration for 360-day tokens)
- Client certificate renewal (automatic when reaching renewal threshold - typically ~6 days before 30-day expiration)
- Credential delivery to managed clusters (automatic via ManifestWork)
- CSR approval (automatic when enabled)

### ⚠️ Action Required Only When

**Manual intervention needed only if:**
- Bootstrap token has expired (check actual expiration in import secret)
- Both bootstrap token AND client certificates expired simultaneously
- Managed cluster remains isolated past the bootstrap token's recorded expiration
- Auto-approval CSR feature disabled (requires manual CSR approval)

**To check if intervention is needed:**
```bash
# Check bootstrap token expiration
kubectl get secret -n <cluster-name> <cluster-name>-import \
  -o jsonpath='{.data.expiration}' | base64 -d
```

### 📊 Recommended Alert Thresholds

Monitor the actual expiration dates recorded in the secrets rather than time since last event.

| Metric | Warning | Critical |
|--------|---------|----------|
| Bootstrap token remaining lifetime | <90 days until expiration | <30 days until expiration |
| Client certificate remaining lifetime | <10 days* until expiration | <3 days* until expiration |
| ManagedCluster unavailable | >5 minutes | >15 minutes |
| Import controller unavailable | >5 minutes | >30 minutes |

**Query commands for monitoring:**
```bash
# Bootstrap token expiration (hub)
kubectl get secret -n <cluster-name> <cluster-name>-import \
  -o jsonpath='{.data.expiration}' | base64 -d

# Client certificate expiration (spoke)
kubectl get secret -n open-cluster-management-agent hub-kubeconfig-secret \
  -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -enddate
```

**\* Adjust based on your hub's `--cluster-signing-duration` setting:**
- For 30-day certificates (720h): Use thresholds above
- For 365-day certificates (8760h): Use <90 days warning, <30 days critical

---

## FAQ

**Q: What happens if I upgrade the hub while tokens are approaching expiration?**  
A: No impact. Token checks continue approximately every 10 minutes during and after upgrades. The controller will renew tokens based on their recorded expiration dates.

**Q: Can I manually force token renewal?**  
A: Yes, add any label/annotation to the ManagedCluster resource to trigger reconciliation.

**Q: Do I need to restart klusterlet for token updates?**  
A: No. The work agent automatically applies token updates. Klusterlet only reads the token when re-bootstrapping.

**Q: What if my managed cluster is offline for weeks?**  
A: When it comes back online, the work agent syncs all pending updates including new tokens. If the client certificates expired during downtime, the klusterlet will automatically re-bootstrap using the bootstrap token to get new certificates.

**Q: How do I verify token renewal is working?**  
A: Check hub controller logs for "token should be refreshed" and "create a new bootstrap kubeconfig" messages. These appear when tokens reach their renewal threshold (72 days before expiration for 360-day tokens). You can also check the token's expiration date directly:
```bash
kubectl get secret -n <cluster-name> <cluster-name>-import \
  -o jsonpath='{.data.expiration}' | base64 -d
```

**Q: Does the bootstrap token expire if I never import new clusters?**  
A: The bootstrap token has a lifetime (typically 360 days) and is automatically renewed when it reaches the renewal threshold (72 days before expiration), regardless of whether new clusters are being imported. If the token expires, it only affects re-bootstrapping scenarios (certificate expiration recovery). Normal certificate rotation continues using the existing valid client certificates.

**Q: What credentials does the klusterlet use for normal operations?**  
A: The klusterlet uses **client certificates** (in `hub-kubeconfig-secret`) for all normal hub communication, including certificate renewal CSRs. The bootstrap token is only used during initial import or when re-establishing credentials after certificate expiration.

**Q: Why does my cluster have different certificate lifetimes than documented?**  
A: Certificate lifetimes are controlled by the hub cluster's `--cluster-signing-duration` flag on kube-controller-manager. The OpenShift default is 30 days (`720h`), but your cluster may be configured differently. Check your configuration with the command in the "Verify Your Configuration" section.

**Q: Can I change the certificate lifetime?**  
A: Yes, but it requires modifying the hub cluster's kube-controller-manager configuration. On OpenShift, this is managed by the cluster configuration operator. Changing this setting will affect all client certificates issued by the cluster, not just OCM. Consult your cluster administrator before making changes.

---

## Support Information

If automatic renewal fails or manual recovery doesn't work:

**Collect diagnostic data:**
   ```bash
   # Hub cluster
   kubectl logs -n multicluster-engine deployment/managedcluster-import-controller-v2
   kubectl get managedcluster <cluster-name> -o yaml
   kubectl get secret -n <cluster-name> <cluster-name>-import \
     -o jsonpath='{.metadata}' | jq
   
   # Managed cluster
   kubectl logs -n open-cluster-management-agent deployment/klusterlet-registration-agent
   kubectl get secret -n open-cluster-management-agent bootstrap-hub-kubeconfig \
     -o jsonpath='{.metadata}' | jq
   ```

   **Security Note:** Only collect Secret metadata (creation time, annotations, labels). Never include `.data` fields in support tickets as they contain sensitive credentials.
