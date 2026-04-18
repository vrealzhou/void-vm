#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TEST_ROOT="${REPO_ROOT}/.cache-test/e2e"
STATE_DIR="${TEST_ROOT}/state"
KNOWN_HOSTS_FILE="${STATE_DIR}/known_hosts"
E2E_BASE_IMAGE="${E2E_BASE_IMAGE:-}"
E2E_SSH_PRIVATE_KEY="${E2E_SSH_PRIVATE_KEY:-${HOME}/.ssh/id_ed25519}"
VM_USER="${E2E_VM_USER:-dev}"
VM_IP="192.168.64.10"

vmctl() {
  (
    cd "${REPO_ROOT}"
    export VMCTL_REPO_ROOT="${REPO_ROOT}"
    go run ./cmd/vmctl "$@"
  )
}

ssh_vm() {
  ssh \
    -i "${E2E_SSH_PRIVATE_KEY}" \
    -o BatchMode=yes \
    -o ConnectTimeout=5 \
    -o UserKnownHostsFile="${KNOWN_HOSTS_FILE}" \
    -o StrictHostKeyChecking=accept-new \
    "${VM_USER}@${VM_IP}" \
    "$@"
}

cleanup() {
  VM_NAME=e2e-vm \
  VM_STATE_DIR="${STATE_DIR}" \
  VM_GUI=0 \
  vmctl stop >/dev/null 2>&1 || true
}

trap cleanup EXIT

rm -rf "${STATE_DIR}"
mkdir -p "${STATE_DIR}"

[[ -f "${E2E_SSH_PRIVATE_KEY}" ]] || { echo "missing private key: ${E2E_SSH_PRIVATE_KEY}" >&2; exit 1; }

start_vm() {
  (
    export VM_NAME=e2e-vm
    export VM_STATE_DIR="${STATE_DIR}"
    export VM_GUI=0
    export VM_CPUS=2
    export VM_MEMORY_MIB=6144
    export VM_DISK_SIZE=24G
    export VM_SSH_USER="${VM_USER}"
    export VM_STATIC_IP="${VM_IP}"
    export VM_SSH_KNOWN_HOSTS_FILE="${KNOWN_HOSTS_FILE}"
    if [[ -n "${E2E_BASE_IMAGE}" ]]; then
      [[ -f "${E2E_BASE_IMAGE}" ]] || { echo "missing base image: ${E2E_BASE_IMAGE}" >&2; exit 1; }
      export VM_BASE_IMAGE="${E2E_BASE_IMAGE}"
    fi
    vmctl start
  )
}

wait_for_ssh() {
  for _ in $(seq 1 120); do
    if ssh_vm true >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  echo "SSH did not become ready in time" >&2
  return 1
}

start_vm
wait_for_ssh

[[ -f "${STATE_DIR}/bootstrap.done" ]] || {
  echo "missing bootstrap marker: ${STATE_DIR}/bootstrap.done" >&2
  exit 1
}
BOOTSTRAP_MARKER="$(cat "${STATE_DIR}/bootstrap.done")"

ssh_vm "fish -lc '
  command -v brew >/tmp/brew-path.txt
  command -v starship >/tmp/starship-path.txt
  command -v hx >/tmp/hx-path.txt
  command -v zellij >/tmp/zellij-path.txt
  command -v zig >/tmp/zig-path.txt
  command -v rustc >/tmp/rustc-path.txt
  command -v cargo >/tmp/cargo-path.txt
  fish --version >/tmp/fish-version.txt
  brew --version >/tmp/brew-version.txt
  starship --version >/tmp/starship-version.txt
  hx --version >/tmp/hx-version.txt
  zellij --version >/tmp/zellij-version.txt
  zig version >/tmp/zig-version.txt
  rustc --version >/tmp/rustc-version.txt
  cargo --version >/tmp/cargo-version.txt
' && \
test -f ~/.config/starship.toml && \
test ! -e ~/.config/oh-my-posh && \
test ! -e ~/.config/fish/conf.d/oh-my-posh.fish && \
grep -q 'starship init fish' ~/.config/fish/conf.d/starship.fish && \
grep -q '.cargo/bin' ~/.config/fish/conf.d/starship.fish && \
grep -q 'brew shellenv' ~/.config/fish/conf.d/starship.fish && \
getent passwd ${VM_USER} | grep -q '/usr/bin/fish' && \
sudo sv status NetworkManager sshd dbus seatd chronyd >/tmp/service-status.txt"

ssh_vm "bash -lc 'nohup chromium --headless --disable-gpu --no-sandbox --remote-debugging-address=0.0.0.0 --remote-debugging-port=18080 about:blank >/tmp/chromium-e2e.log 2>&1 & echo \$! >/tmp/chromium-e2e.pid'"

for _ in $(seq 1 30); do
  if curl --fail "http://${VM_IP}:18080/json/version" | grep -q 'Browser'; then
    break
  fi
  sleep 2
done
curl --fail "http://${VM_IP}:18080/json/version" | grep -q 'Browser'

VM_NAME=e2e-vm \
VM_STATE_DIR="${STATE_DIR}" \
VM_GUI=0 \
vmctl stop

VM_NAME=e2e-vm \
VM_STATE_DIR="${STATE_DIR}" \
VM_GUI=0 \
VM_SSH_USER="${VM_USER}" \
VM_STATIC_IP="${VM_IP}" \
VM_SSH_KNOWN_HOSTS_FILE="${KNOWN_HOSTS_FILE}" \
vmctl start

wait_for_ssh

[[ "$(cat "${STATE_DIR}/bootstrap.done")" == "${BOOTSTRAP_MARKER}" ]] || {
  echo "bootstrap marker changed after restart; bootstrap re-ran unexpectedly" >&2
  exit 1
}

ssh_vm "fish -lc 'command -v brew >/dev/null && command -v hx >/dev/null && command -v zellij >/dev/null && command -v zig >/dev/null && command -v rustc >/dev/null && command -v cargo >/dev/null'"

VM_NAME=e2e-vm \
VM_STATE_DIR="${STATE_DIR}" \
VM_GUI=0 \
vmctl stop

printf 'E2E passed\n'
