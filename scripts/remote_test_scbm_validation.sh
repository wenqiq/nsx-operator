#!/usr/bin/env bash
# ==============================================================================
# SubnetConnectionBindingMap (SCBM) Validation Automated Test Script
#
# This script is executed on the Target Supervisor node (or via SSH) to:
# 1. Ensure /etc/vmware/wcp/tls/manager permissions
# 2. Apply Subnet & SCBM CRDs
# 3. Restart nsx-ncp deployment (nsx-operator)
# 4. Create test Subnets (parent-v4: IPv4, child-v4: IPv4, child-v6: IPv6)
# 5. Apply SCBMs:
#    - scbm-v4-to-v4 (Valid IPv4-IPv4 binding -> Ready: True)
#    - scbm-neg-v6-to-v4 (Incompatible IPv6-IPv4 binding -> Ready: False)
# 6. Verify validation result and collect artifacts into /root/scbm-validation/
# ==============================================================================

set -e

TEST_DIR="/root/scbm-validation"
KUBECONFIG="/etc/kubernetes/admin.conf"
NAMESPACE="${1:-ns-1}"

echo "=== 1. Preparing Test Directory & Namespace ==="
mkdir -p "${TEST_DIR}"
kubectl --kubeconfig="${KUBECONFIG}" create namespace "${NAMESPACE}" 2>/dev/null || true

echo "=== 2. Checking manager binary permissions ==="
if [ -f "/etc/vmware/wcp/tls/manager" ]; then
    chmod +x /etc/vmware/wcp/tls/manager
    ls -l /etc/vmware/wcp/tls/manager
else
    echo "Warning: /etc/vmware/wcp/tls/manager not found!"
fi

echo "=== 3. Applying Subnet & SCBM CRDs ==="
if [ -f "${TEST_DIR}/crd.nsx.vmware.com_subnetconnectionbindingmaps.yaml" ]; then
    kubectl --kubeconfig="${KUBECONFIG}" apply -f "${TEST_DIR}/crd.nsx.vmware.com_subnetconnectionbindingmaps.yaml"
fi
if [ -f "${TEST_DIR}/crd.nsx.vmware.com_subnets.yaml" ]; then
    kubectl --kubeconfig="${KUBECONFIG}" apply -f "${TEST_DIR}/crd.nsx.vmware.com_subnets.yaml"
fi

echo "=== 4. Restarting nsx-ncp Deployment ==="
kubectl --kubeconfig="${KUBECONFIG}" rollout restart deployment nsx-ncp -n vmware-system-nsx
echo "Waiting for nsx-ncp rollout to complete..."
kubectl --kubeconfig="${KUBECONFIG}" rollout status deployment nsx-ncp -n vmware-system-nsx --timeout=180s

echo "Waiting for webhook service endpoints..."
for i in {1..30}; do
    ENDPOINTS=$(kubectl --kubeconfig="${KUBECONFIG}" get endpoints vmware-system-nsx-operator-webhook-service -n vmware-system-nsx -o jsonpath='{.subsets[*].addresses[*].ip}' 2>/dev/null || true)
    if [ -n "${ENDPOINTS}" ]; then
        echo "Webhook endpoints ready: ${ENDPOINTS}"
        break
    fi
    echo "Waiting for webhook endpoints... ($i/30)"
    sleep 2
done
sleep 5

echo "=== 5. Generating Subnet & SCBM Test Manifests ==="
cat << EOF > "${TEST_DIR}/test-subnets.yaml"
apiVersion: crd.nsx.vmware.com/v1alpha1
kind: Subnet
metadata:
  name: parent-v4
spec:
  accessMode: Private
  ipAddressType: IPv4
  ipv4SubnetSize: 32
---
apiVersion: crd.nsx.vmware.com/v1alpha1
kind: Subnet
metadata:
  name: child-v4
spec:
  accessMode: Private
  ipAddressType: IPv4
  ipv4SubnetSize: 32
---
apiVersion: crd.nsx.vmware.com/v1alpha1
kind: Subnet
metadata:
  name: child-v6
spec:
  accessMode: Private
  ipAddressType: IPv6
  ipv6PrefixLength: 112
EOF

cat << EOF > "${TEST_DIR}/test-scbm-valid.yaml"
apiVersion: crd.nsx.vmware.com/v1alpha1
kind: SubnetConnectionBindingMap
metadata:
  name: scbm-v4-to-v4
spec:
  subnetAssociation: Trunk
  subnetName: child-v4
  targetSubnetName: parent-v4
EOF

cat << EOF > "${TEST_DIR}/test-scbm-incompatible.yaml"
apiVersion: crd.nsx.vmware.com/v1alpha1
kind: SubnetConnectionBindingMap
metadata:
  name: scbm-neg-v6-to-v4
spec:
  subnetAssociation: Trunk
  subnetName: child-v6
  targetSubnetName: parent-v4
EOF

echo "=== 6. Applying Test Subnets ==="
APPLY_SUBNET_SUCCESS=false
for i in {1..6}; do
    if kubectl --kubeconfig="${KUBECONFIG}" apply -f "${TEST_DIR}/test-subnets.yaml" -n "${NAMESPACE}"; then
        APPLY_SUBNET_SUCCESS=true
        break
    fi
    echo "Applying subnets failed, retrying in 5s ($i/6)..."
    sleep 5
done
if [ "${APPLY_SUBNET_SUCCESS}" = "false" ]; then
    echo "Error: Failed to apply test subnets after retries."
    exit 1
fi
echo "Waiting for Subnets to be realized..."
sleep 5

echo "=== 7. Applying SCBM Test Resources ==="
kubectl --kubeconfig="${KUBECONFIG}" apply -f "${TEST_DIR}/test-scbm-valid.yaml" -n "${NAMESPACE}"
kubectl --kubeconfig="${KUBECONFIG}" apply -f "${TEST_DIR}/test-scbm-incompatible.yaml" -n "${NAMESPACE}"
echo "Waiting for SCBM reconciliation..."
sleep 5

echo "=== 8. Collecting Status & Logs ==="
kubectl --kubeconfig="${KUBECONFIG}" get subnets -n "${NAMESPACE}" -o wide > "${TEST_DIR}/subnets_status.txt" 2>&1 || true
kubectl --kubeconfig="${KUBECONFIG}" get subnetconnectionbindingmaps -n "${NAMESPACE}" -o yaml > "${TEST_DIR}/scbm_cr_status.txt" 2>&1 || true
kubectl --kubeconfig="${KUBECONFIG}" logs -n vmware-system-nsx -l component=nsx-ncp -c nsx-operator --tail=300 > "${TEST_DIR}/nsx_operator.log" 2>&1 || true

echo "=== 9. Verification of Validation Results ==="
VALID_STATUS=$(kubectl --kubeconfig="${KUBECONFIG}" get scbm scbm-v4-to-v4 -n "${NAMESPACE}" -o jsonpath='{.status.conditions[0].status}' 2>/dev/null || echo "Unknown")
INCOMPATIBLE_STATUS=$(kubectl --kubeconfig="${KUBECONFIG}" get scbm scbm-neg-v6-to-v4 -n "${NAMESPACE}" -o jsonpath='{.status.conditions[0].status}' 2>/dev/null || echo "Unknown")
INCOMPATIBLE_MSG=$(kubectl --kubeconfig="${KUBECONFIG}" get scbm scbm-neg-v6-to-v4 -n "${NAMESPACE}" -o jsonpath='{.status.conditions[0].message}' 2>/dev/null || echo "")

cat << EOF > "${TEST_DIR}/test_summary.txt"
================================================================================
SCBM IPAddressType Validation Test Summary
================================================================================
Timestamp: $(date)
Namespace: ${NAMESPACE}
Target Directory: ${TEST_DIR}

Results:
1. Valid IPv4-IPv4 Binding (scbm-v4-to-v4):
   - Expected Ready Status: True
   - Actual Ready Status: ${VALID_STATUS}

2. Incompatible IPv6-IPv4 Binding (scbm-neg-v6-to-v4):
   - Expected Ready Status: False
   - Actual Ready Status: ${INCOMPATIBLE_STATUS}
   - Failure Message: ${INCOMPATIBLE_MSG}

Files Generated:
- test-subnets.yaml
- test-scbm-valid.yaml
- test-scbm-incompatible.yaml
- subnets_status.txt
- scbm_cr_status.txt
- nsx_operator.log
- test_summary.txt
================================================================================
EOF

cat "${TEST_DIR}/test_summary.txt"

if [ "${VALID_STATUS}" = "True" ] && [ "${INCOMPATIBLE_STATUS}" = "False" ]; then
    echo "SUCCESS: SCBM IPAddressType Validation test passed!"
else
    echo "FAILURE: Validation result did not match expectations!"
    exit 1
fi

