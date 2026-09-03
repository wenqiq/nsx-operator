#!/usr/bin/env python3
"""
Automated Test Script for DNSRecord Feature on Local Testbed / Supervisor Node.

Usage:
  python3 scripts/test_dnsrecord.py

Environment Variables (Optional):
  DEV_HOST          - Dev machine IP (default: 10.167.225.61)
  DEV_USER          - Dev machine SSH user (default: wq026466)
  DEV_PASS          - Dev machine SSH password (default: Admin!23)
  DEV_BINARY_PATH   - Path to manager binary on Dev machine (default: /home/wq026466/go/src/nsx-operator/bin/manager)
  TARGET_HOST       - Target Supervisor node IP (default: 10.162.214.213)
  TARGET_USER       - Target node SSH user (default: root)
  TARGET_PASS       - Target node SSH password (default: mfn^d{2lIN2nMFec)
  NAMESPACE         - Test namespace on Target node (default: test-ns)
"""

import os
import sys
import time
import json
import pty

DEV_HOST = os.environ.get("DEV_HOST", "10.167.225.61")
DEV_USER = os.environ.get("DEV_USER", "wq026466")
DEV_PASS = os.environ.get("DEV_PASS", "Admin!23")
DEV_BINARY_PATH = os.environ.get("DEV_BINARY_PATH", "/home/wq026466/go/src/nsx-operator/bin/manager")

TARGET_HOST = os.environ.get("TARGET_HOST", "10.162.214.213")
TARGET_USER = os.environ.get("TARGET_USER", "root")
TARGET_PASS = os.environ.get("TARGET_PASS", "mfn^d{2lIN2nMFec")
TARGET_BINARY_PATH = "/etc/vmware/wcp/tls/manager"
TEST_DIR = "/root/dns-record"
KUBECONFIG = "/etc/kubernetes/admin.conf"
NAMESPACE = os.environ.get("NAMESPACE", "test-ns")


def run_cmd(cmd_list, password, timeout=300):
    pid, fd = pty.fork()
    if pid == 0:
        os.execvp(cmd_list[0], cmd_list)
    else:
        password_sent = False
        output = []
        start_time = time.time()
        while True:
            try:
                data = os.read(fd, 4096)
                if not data:
                    break
                output.append(data)
                if not password_sent and b"password" in data.lower():
                    os.write(fd, (password + "\n").encode())
                    password_sent = True
            except OSError:
                break
            if time.time() - start_time > timeout:
                os.kill(pid, 9)
                break
        os.waitpid(pid, 0)
        return b"".join(output).decode("utf-8", errors="ignore")


def run_ssh(host, user, password, command, timeout=300):
    cmd = ["ssh", "-o", "StrictHostKeyChecking=no", f"{user}@{host}", command]
    return run_cmd(cmd, password, timeout)


def run_scp(src, dst, password, timeout=300):
    cmd = ["scp", "-q", "-o", "StrictHostKeyChecking=no", src, dst]
    return run_cmd(cmd, password, timeout)


def main():
    print("=== Step 1: Downloading manager binary from Dev machine ===")
    local_tmp_bin = "/tmp/manager_binary_test"
    run_scp(f"{DEV_USER}@{DEV_HOST}:{DEV_BINARY_PATH}", local_tmp_bin, DEV_PASS, timeout=300)
    if not os.path.exists(local_tmp_bin) or os.path.getsize(local_tmp_bin) == 0:
        print(f"Failed to fetch binary from {DEV_HOST}:{DEV_BINARY_PATH}")
        sys.exit(1)
    bin_size = os.path.getsize(local_tmp_bin)
    print(f"Successfully downloaded manager binary ({bin_size} bytes).")

    print("\n=== Step 2: Uploading manager binary to Target machine ===")
    run_scp(local_tmp_bin, f"{TARGET_USER}@{TARGET_HOST}:{TARGET_BINARY_PATH}", TARGET_PASS, timeout=300)
    print(run_ssh(TARGET_HOST, TARGET_USER, TARGET_PASS, f"chmod +x {TARGET_BINARY_PATH} && ls -l {TARGET_BINARY_PATH}"))

    print("\n=== Step 3: Preparing test directory & updating nsx-ncp-config ConfigMap ===")
    run_ssh(TARGET_HOST, TARGET_USER, TARGET_PASS, f"mkdir -p {TEST_DIR}")

    # Check & fix ConfigMap for cluster = k8scluster
    cm_raw = run_ssh(TARGET_HOST, TARGET_USER, TARGET_PASS, f"kubectl --kubeconfig={KUBECONFIG} get configmap nsx-ncp-config -n vmware-system-nsx -o json")
    start_idx = cm_raw.find("{")
    if start_idx != -1:
        try:
            data = json.loads(cm_raw[start_idx:])
            ncp_ini = data["data"]["ncp.ini"]
            if "cluster = k8scluster" not in ncp_ini:
                print("Updating nsx-ncp-config ConfigMap to enable cluster = k8scluster...")
                new_ncp_ini = ncp_ini.replace("#cluster = k8scluster", "cluster = k8scluster")
                if "cluster = k8scluster" not in new_ncp_ini:
                    new_ncp_ini = new_ncp_ini.replace("[coe]", "[coe]\ncluster = k8scluster")
                data["data"]["ncp.ini"] = new_ncp_ini
                local_cm_file = "/tmp/nsx_ncp_cm_patch.json"
                with open(local_cm_file, "w") as f:
                    json.dump(data, f)
                run_scp(local_cm_file, f"{TARGET_USER}@{TARGET_HOST}:/tmp/cm_patch.json", TARGET_PASS)
                run_ssh(TARGET_HOST, TARGET_USER, TARGET_PASS, f"kubectl --kubeconfig={KUBECONFIG} apply -f /tmp/cm_patch.json")
        except Exception as e:
            print(f"Warning updating ConfigMap: {e}")

    print("\n=== Step 4: Installing DNSRecord CRD ===")
    crd_local_path = "build/yaml/crd/vpc/crd.nsx.vmware.com_dnsrecords.yaml"
    if os.path.exists(crd_local_path):
        run_scp(crd_local_path, f"{TARGET_USER}@{TARGET_HOST}:{TEST_DIR}/crd_dnsrecords.yaml", TARGET_PASS)
        run_ssh(TARGET_HOST, TARGET_USER, TARGET_PASS, f"kubectl --kubeconfig={KUBECONFIG} apply -f {TEST_DIR}/crd_dnsrecords.yaml")

    print("\n=== Step 5: Restarting nsx-ncp deployment ===")
    run_ssh(TARGET_HOST, TARGET_USER, TARGET_PASS, f"kubectl --kubeconfig={KUBECONFIG} rollout restart deployment nsx-ncp -n vmware-system-nsx")
    time.sleep(10)

    print("\n=== Step 6: Creating test manifests in /root/dns-record/ ===")
    manifest_cmd = f"""
cat << 'EOF' > {TEST_DIR}/dnsrecord_a.yaml
apiVersion: crd.nsx.vmware.com/v1alpha1
kind: DNSRecord
metadata:
  name: test-dnsrecord-a
  namespace: {NAMESPACE}
spec:
  domainName: example.com
  recordName: test-a
  recordType: A
  recordValues:
    - 10.0.0.100
  ttl: 300
EOF

cat << 'EOF' > {TEST_DIR}/dnsrecord_cname.yaml
apiVersion: crd.nsx.vmware.com/v1alpha1
kind: DNSRecord
metadata:
  name: test-dnsrecord-cname
  namespace: {NAMESPACE}
spec:
  domainName: example.com
  recordName: test-cname
  recordType: CNAME
  recordValues:
    - target.example.com
  ttl: 600
EOF

cat << 'EOF' > {TEST_DIR}/dnsrecord_apex.yaml
apiVersion: crd.nsx.vmware.com/v1alpha1
kind: DNSRecord
metadata:
  name: test-dnsrecord-apex
  namespace: {NAMESPACE}
spec:
  domainName: example.com
  recordName: "@"
  recordType: A
  recordValues:
    - 10.0.0.101
  ttl: 300
EOF
"""
    run_ssh(TARGET_HOST, TARGET_USER, TARGET_PASS, manifest_cmd)

    print("\n=== Step 7: Applying test manifests and verifying ===")
    run_ssh(TARGET_HOST, TARGET_USER, TARGET_PASS, f"kubectl --kubeconfig={KUBECONFIG} apply -f {TEST_DIR}/dnsrecord_a.yaml")
    run_ssh(TARGET_HOST, TARGET_USER, TARGET_PASS, f"kubectl --kubeconfig={KUBECONFIG} apply -f {TEST_DIR}/dnsrecord_cname.yaml")
    run_ssh(TARGET_HOST, TARGET_USER, TARGET_PASS, f"kubectl --kubeconfig={KUBECONFIG} apply -f {TEST_DIR}/dnsrecord_apex.yaml")

    time.sleep(5)

    print("\n=== Step 8: Collecting logs and statuses into /root/dns-record/ ===")
    collect_cmd = f"""
kubectl --kubeconfig={KUBECONFIG} get crd dnsrecords.crd.nsx.vmware.com -o yaml > {TEST_DIR}/crd_status.txt
kubectl --kubeconfig={KUBECONFIG} get dnsrecords -A -o yaml > {TEST_DIR}/dnsrecord_cr_status.txt
kubectl --kubeconfig={KUBECONFIG} logs -n vmware-system-nsx -l component=nsx-ncp -c nsx-operator --tail=200 > {TEST_DIR}/nsx_operator.log
kubectl --kubeconfig={KUBECONFIG} logs -n vmware-system-nsx -l component=nsx-ncp -c nsx-ncp --tail=200 > {TEST_DIR}/nsx_ncp.log

cat << 'EOF' > {TEST_DIR}/test_summary.txt
================================================================================
DNS Record Automated Test Summary
================================================================================
Source Machine: {DEV_USER}@{DEV_HOST}:{DEV_BINARY_PATH}
Target Machine: {TARGET_USER}@{TARGET_HOST}:{TARGET_BINARY_PATH}
Test Directory: {TEST_DIR}

Files in {TEST_DIR}:
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
"""
    run_ssh(TARGET_HOST, TARGET_USER, TARGET_PASS, collect_cmd)

    print("\n=== Test execution completed! Artifacts saved in /root/dns-record/ ===")
    print(run_ssh(TARGET_HOST, TARGET_USER, TARGET_PASS, f"ls -la {TEST_DIR}/"))


if __name__ == "__main__":
    main()
