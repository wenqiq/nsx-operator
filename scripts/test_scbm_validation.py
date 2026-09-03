#!/usr/bin/env python3
"""
Automated Test Script for SubnetConnectionBindingMap (SCBM) Validation Feature on Local Testbed / Supervisor Node.

Usage:
  python3 scripts/test_scbm_validation.py

Environment Variables (Optional):
  DEV_HOST          - Dev machine IP (default: 10.167.225.61)
  DEV_USER          - Dev machine SSH user (default: wq026466)
  DEV_PASS          - Dev machine SSH password (default: Admin!23)
  DEV_BINARY_PATH   - Path to manager binary on Dev machine (default: /home/wq026466/go/src/nsx-operator/bin/manager)
  TARGET_HOST       - Target Supervisor node IP (default: 10.162.214.213)
  TARGET_USER       - Target node SSH user (default: root)
  TARGET_PASS       - Target node SSH password (default: mfn^d{2lIN2nMFec)
  NAMESPACE         - Test namespace on Target node (default: ns-1)
"""

import os
import sys
import time
import pty

SKIP_BINARY = os.environ.get("SKIP_BINARY", "true").lower() in ("true", "1", "yes")
DEV_HOST = os.environ.get("DEV_HOST", "10.167.225.61")
DEV_USER = os.environ.get("DEV_USER", "wq026466")
DEV_PASS = os.environ.get("DEV_PASS", "Admin!23")
DEV_BINARY_PATH = os.environ.get("DEV_BINARY_PATH", "/home/wq026466/go/src/nsx-operator/bin/manager")

TARGET_HOST = os.environ.get("TARGET_HOST", "10.162.200.223")
TARGET_USER = os.environ.get("TARGET_USER", "root")
TARGET_PASS = os.environ.get("TARGET_PASS", "gN}^PdZ8yD'gq4OY")
TARGET_BINARY_PATH = "/etc/vmware/wcp/tls/manager"
TEST_DIR = "/root/scbm-validation"
KUBECONFIG = "/etc/kubernetes/admin.conf"
NAMESPACE = os.environ.get("NAMESPACE", "ns-1")


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
    if not SKIP_BINARY:
        print("=== Step 1: Downloading manager binary from Dev machine ===")
        local_tmp_bin = "/tmp/manager_binary_scbm_test"
        run_scp(f"{DEV_USER}@{DEV_HOST}:{DEV_BINARY_PATH}", local_tmp_bin, DEV_PASS, timeout=300)
        if not os.path.exists(local_tmp_bin) or os.path.getsize(local_tmp_bin) == 0:
            print(f"Failed to fetch binary from {DEV_HOST}:{DEV_BINARY_PATH}")
            sys.exit(1)
        bin_size = os.path.getsize(local_tmp_bin)
        print(f"Successfully downloaded manager binary ({bin_size} bytes).")

        print("\n=== Step 2: Uploading manager binary to Target machine ===")
        run_scp(local_tmp_bin, f"{TARGET_USER}@{TARGET_HOST}:{TARGET_BINARY_PATH}", TARGET_PASS, timeout=300)
        print(run_ssh(TARGET_HOST, TARGET_USER, TARGET_PASS, f"chmod +x {TARGET_BINARY_PATH} && ls -l {TARGET_BINARY_PATH}"))
    else:
        print("=== Step 1 & 2: Skipped binary download and upload (SKIP_BINARY=true) ===")

    print("\n=== Step 3: Preparing test directory on Target machine ===")
    run_ssh(TARGET_HOST, TARGET_USER, TARGET_PASS, f"mkdir -p {TEST_DIR}")

    print("\n=== Step 4: Uploading CRDs and Test Shell Script ===")
    scbm_crd_path = "build/yaml/crd/vpc/crd.nsx.vmware.com_subnetconnectionbindingmaps.yaml"
    subnet_crd_path = "build/yaml/crd/vpc/crd.nsx.vmware.com_subnets.yaml"
    remote_script_path = "scripts/remote_test_scbm_validation.sh"

    if os.path.exists(scbm_crd_path):
        run_scp(scbm_crd_path, f"{TARGET_USER}@{TARGET_HOST}:{TEST_DIR}/crd.nsx.vmware.com_subnetconnectionbindingmaps.yaml", TARGET_PASS)
    if os.path.exists(subnet_crd_path):
        run_scp(subnet_crd_path, f"{TARGET_USER}@{TARGET_HOST}:{TEST_DIR}/crd.nsx.vmware.com_subnets.yaml", TARGET_PASS)
    if os.path.exists(remote_script_path):
        run_scp(remote_script_path, f"{TARGET_USER}@{TARGET_HOST}:{TEST_DIR}/remote_test_scbm_validation.sh", TARGET_PASS)

    print("\n=== Step 5: Executing Remote Test Script on Target Node ===")
    output = run_ssh(TARGET_HOST, TARGET_USER, TARGET_PASS, f"chmod +x {TEST_DIR}/remote_test_scbm_validation.sh && {TEST_DIR}/remote_test_scbm_validation.sh {NAMESPACE}")
    print(output)

    print("\n=== Test Execution Complete ===")


if __name__ == "__main__":
    main()

