#!/usr/bin/env bash
# ==============================================================================
# DNSRecord Local Testbed Automated Test Script
#
# This script is executed on the Target Supervisor node (or via SSH) to:
# 1. Ensure /etc/vmware/wcp/tls/manager permissions
# 2. Check and fix nsx-ncp-config ConfigMap (cluster = k8scluster)
# 3. Apply DNSRecord CRD
# 4. Restart nsx-ncp deployment
# 5. Create and apply test manifests for DNSRecord (A, CNAME, Apex)
# 6. Collect logs and status outputs into /root/dns-record/
# ==============================================================================

set -e

TEST_DIR="/root/dns-record"
KUBECONFIG="/etc/kubernetes/admin.conf"
NAMESPACE="${1:-test-ns}"

echo "=== 1. Preparing Test Directory ==="
mkdir -p "${TEST_DIR}"

echo "=== 2. Checking manager binary permissions ==="
if [ -f "/etc/vmware/wcp/tls/manager" ]; then
    chmod +x /etc/vmware/wcp/tls/manager
    ls -l /etc/vmware/wcp/tls/manager
else
    echo "Warning: /etc/vmware/wcp/tls/manager not found!"
fi

echo "=== 3. Ensuring nsx-ncp-config ConfigMap settings ==="
if kubectl --kubeconfig="${KUBECONFIG}" get configmap nsx-ncp-config -n vmware-system-nsx >/dev/null 2>&1; then
    ncp_ini=$(kubectl --kubeconfig="${KUBECONFIG}" get configmap nsx-ncp-config -n vmware-system-nsx -o jsonpath='{.data.ncp\.ini}')
    if ! echo "${ncp_ini}" | grep -q "^cluster = k8scluster"; then
        echo "Updating nsx-ncp-config ConfigMap to set cluster = k8scluster..."
        python3 -c '
import json, subprocess
raw = subprocess.check_output(["kubectl", "--kubeconfig=/etc/kubernetes/admin.conf", "get", "configmap", "nsx-ncp-config", "-n", "vmware-system-nsx", "-o", "json"])
data = json.loads(raw)
ini = data["data"]["ncp.ini"]
if "cluster = k8scluster" not in ini:
    ini = ini.replace("#cluster = k8scluster", "cluster = k8scluster")
    if "cluster = k8scluster" not in ini:
        ini = ini.replace("[coe]", "[coe]\ncluster = k8scluster")
    data["data"]["ncp.ini"] = ini
    with open("/tmp/cm_fix.json", "w") as f:
        json.dump(data, f)
    subprocess.check_call(["kubectl", "--kubeconfig=/etc/kubernetes/admin.conf", "apply", "-f", "/tmp/cm_fix.json"])
'
    else
        echo "nsx-ncp-config already contains cluster = k8scluster."
    fi
fi

echo "=== 4. Applying DNSRecord CRD ==="
if [ -f "${TEST_DIR}/crd_dnsrecords.yaml" ]; then
    kubectl --kubeconfig="${KUBECONFIG}" apply -f "${TEST_DIR}/crd_dnsrecords.yaml"
fi

echo "=== 5. Restarting nsx-ncp Deployment ==="
kubectl --kubeconfig="${KUBECONFIG}" rollout restart deployment nsx-ncp -n vmware-system-nsx

echo "=== 6. Generating Test Manifests in ${TEST_DIR} ==="
cat << EOF > "${TEST_DIR}/dnsrecord_a.yaml"
apiVersion: crd.nsx.vmware.com/v1alpha1
kind: DNSRecord
metadata:
  name: test-dnsrecord-a
  namespace: ${NAMESPACE}
spec:
  domainName: example.com
  recordName: test-a
  recordType: A
  recordValues:
    - 10.0.0.100
  ttl: 300
EOF

cat << EOF > "${TEST_DIR}/dnsrecord_cname.yaml"
apiVersion: crd.nsx.vmware.com/v1alpha1
kind: DNSRecord
metadata:
  name: test-dnsrecord-cname
  namespace: ${NAMESPACE}
spec:
  domainName: example.com
  recordName: test-cname
  recordType: CNAME
  recordValues:
    - target.example.com
  ttl: 600
EOF

cat << EOF > "${TEST_DIR}/dnsrecord_apex.yaml"
apiVersion: crd.nsx.vmware.com/v1alpha1
kind: DNSRecord
metadata:
  name: test-dnsrecord-apex
  namespace: ${NAMESPACE}
spec:
  domainName: example.com
  recordName: "@"
  recordType: A
  recordValues:
    - 10.0.0.101
  ttl: 300
EOF

echo "=== 7. Applying DNSRecord Test Resources ==="
kubectl --kubeconfig="${KUBECONFIG}" apply -f "${TEST_DIR}/dnsrecord_a.yaml"
kubectl --kubeconfig="${KUBECONFIG}" apply -f "${TEST_DIR}/dnsrecord_cname.yaml"
kubectl --kubeconfig="${KUBECONFIG}" apply -f "${TEST_DIR}/dnsrecord_apex.yaml"

sleep 5

echo "=== 8. Collecting Status & Logs ==="
kubectl --kubeconfig="${KUBECONFIG}" get crd dnsrecords.crd.nsx.vmware.com -o yaml > "${TEST_DIR}/crd_status.txt" 2>&1 || true
kubectl --kubeconfig="${KUBECONFIG}" get dnsrecords -A -o yaml > "${TEST_DIR}/dnsrecord_cr_status.txt" 2>&1 || true
kubectl --kubeconfig="${KUBECONFIG}" logs -n vmware-system-nsx -l component=nsx-ncp -c nsx-operator --tail=200 > "${TEST_DIR}/nsx_operator.log" 2>&1 || true
kubectl --kubeconfig="${KUBECONFIG}" logs -n vmware-system-nsx -l component=nsx-ncp -c nsx-ncp --tail=200 > "${TEST_DIR}/nsx_ncp.log" 2>&1 || true

cat << EOF > "${TEST_DIR}/test_summary.txt"
================================================================================
DNS Record Automated Test Summary
================================================================================
Timestamp: $(date)
Target Directory: ${TEST_DIR}
Namespace: ${NAMESPACE}

Files Generated:
- crd_dnsrecords.yaml
- dnsrecord_a.yaml
- dnsrecord_cname.yaml
- dnsrecord_apex.yaml
- crd_status.txt
- dnsrecord_cr_status.txt
- nsx_operator.log
- nsx_ncp.log
- test_summary.txt
================================================================================
EOF

echo "=== Test Automation Completed! Artifacts saved in ${TEST_DIR}: ==="
ls -la "${TEST_DIR}/"
