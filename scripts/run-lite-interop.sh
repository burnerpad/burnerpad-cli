#!/bin/bash
# Run the real Lite/browser/CLI matrix while observing every IP packet from
# the client process tree. This is intentionally Linux-only: iptables' owner
# match sees the fixed host UID because Docker uses the host user namespace.
set -euo pipefail
export LC_ALL=C

readonly client_uid=60000
readonly run_token=$BASHPID
readonly chain4="BPCLI4_$run_token"
readonly chain6="BPCLI6_$run_token"
readonly container_name="burnerpad-cli-interop-$run_token"
readonly probe_name="burnerpad-cli-egress-probe-$run_token"
readonly playwright_image='mcr.microsoft.com/playwright:v1.62.1-noble@sha256:dcc5531e97840b9b5e794f2814476b21571c5124a3fca2267d73041f56e7580e'

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
cd "$script_dir/.."
readonly repo_root=$PWD

if [[ $(uname -s) != Linux ]]; then
  echo "Lite interoperability egress observation requires Linux" >&2
  exit 1
fi
: "${RUNNER_TEMP:?RUNNER_TEMP is required}"
if [[ $RUNNER_TEMP != /* ]] || [[ ! -d $RUNNER_TEMP ]]; then
  echo "RUNNER_TEMP must name an existing absolute directory" >&2
  exit 1
fi
for command_name in curl docker getent go ip6tables iptables mix sudo; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "required command is unavailable: $command_name" >&2
    exit 1
  fi
done
if ! sudo -n true; then
  echo "non-interactive firewall administration is unavailable" >&2
  exit 1
fi
if getent passwd "$client_uid" >/dev/null; then
  echo "reserved interop UID is assigned to a host account" >&2
  exit 1
fi

# xt_owner uses the socket's saved credentials, not merely the process EUID.
# Refuse every real/effective/saved/filesystem UID collision and every extant
# Internet socket already attributed to the reserved UID.
for status_file in /proc/[0-9]*/status; do
  if uid_fields="$(awk '/^Uid:/ { print $2, $3, $4, $5; found = 1 } END { if (!found) exit 1 }' "$status_file" 2>/dev/null)"; then
    read -r -a uid_values <<<"$uid_fields"
    for uid_value in "${uid_values[@]}"; do
      if [[ $uid_value == "$client_uid" ]]; then
        echo "reserved interop UID appears in an existing process credential" >&2
        exit 1
      fi
    done
  elif [[ -e $status_file ]]; then
    echo "could not inspect an existing process credential" >&2
    exit 1
  fi
done
for socket_table in tcp tcp6 udp udp6 raw raw6 icmp icmp6; do
  socket_path="/proc/net/$socket_table"
  if [[ ! -r $socket_path ]]; then
    echo "could not inspect host Internet sockets" >&2
    exit 1
  fi
  if awk -v uid="$client_uid" 'NR > 1 && $8 == uid { found = 1 } END { exit !found }' "$socket_path"; then
    echo "reserved interop UID already owns an Internet socket" >&2
    exit 1
  fi
done
if ! docker version --format '{{.Server.Version}}' >/dev/null; then
  echo "Docker daemon is unavailable" >&2
  exit 1
fi
for stale_container in "$container_name" "$probe_name"; do
  if docker container inspect "$stale_container" >/dev/null 2>&1; then
    echo "reserved interop container already exists: $stale_container" >&2
    exit 1
  fi
done
for family in iptables ip6tables; do
  chain=$chain4
  [[ $family == ip6tables ]] && chain=$chain6
  if ! sudo -n "$family" -w 5 -S OUTPUT >/dev/null; then
    echo "could not inspect host firewall policy" >&2
    exit 1
  fi
  if sudo -n "$family" -w 5 -S "$chain" >/dev/null 2>&1; then
    echo "reserved interop firewall chain already exists: $chain" >&2
    exit 1
  fi
done

run_dir="$(mktemp -d "$RUNNER_TEMP/burnerpad-cli-interop.XXXXXX")"
readonly run_dir
readonly cli_bin="$run_dir/burnerpad"
readonly server_log="$run_dir/lite.log"
readonly expiry_server_log="$run_dir/lite-expiry.log"
readonly expiry_ready="$run_dir/lite-expiry.ready"
readonly interop_spec="$repo_root/burnerpad-lite/test/browser/cli-interop.spec.mjs"

server_pid=
expiry_server_pid=
interop_spec_intent=0
container_intent=0
probe_intent=0
chain4_intent=0
chain6_intent=0
jump4_intent=0
jump6_intent=0
cleanup_failed=0

# shellcheck disable=SC2329 # Also reached through the EXIT-trap cleanup function.
remove_container() {
  local name=$1
  local existing name_list removal_output
  if removal_output="$(docker rm -f "$name" 2>&1)"; then
    return 0
  fi
  if name_list="$(docker container ls -a --format '{{.Names}}' 2>&1)"; then
    while IFS= read -r existing; do
      if [[ $existing == "$name" ]]; then
        printf 'could not stop %s: %s\n' "$name" "$removal_output" >&2
        cleanup_failed=1
        return 1
      fi
    done <<<"$name_list"
    return 0
  fi
  printf 'could not confirm %s stopped: %s; %s\n' "$name" "$removal_output" "$name_list" >&2
  cleanup_failed=1
  return 1
}

# shellcheck disable=SC2329 # Reached through the EXIT-trap cleanup function.
remove_firewall_family() {
  local family=$1
  local chain=$2
  local jump_intent=$3
  local chain_intent=$4

  if (( jump_intent == 0 && chain_intent == 0 )); then
    return
  fi
  if ! sudo -n "$family" -w 5 -S OUTPUT >/dev/null; then
    echo "could not inspect $family during cleanup" >&2
    cleanup_failed=1
    return
  fi
  if (( jump_intent )) && sudo -n "$family" -w 5 -C OUTPUT -m owner --uid-owner "$client_uid" -j "$chain"; then
    sudo -n "$family" -w 5 -D OUTPUT -m owner --uid-owner "$client_uid" -j "$chain" || cleanup_failed=1
  fi
  if (( chain_intent )) && sudo -n "$family" -w 5 -S "$chain" >/dev/null 2>&1; then
    sudo -n "$family" -w 5 -F "$chain" || cleanup_failed=1
    sudo -n "$family" -w 5 -X "$chain" || cleanup_failed=1
  fi
}

# shellcheck disable=SC2329 # Invoked indirectly by the EXIT trap below.
cleanup() {
  status=$?
  local containers_stopped=1
  trap - EXIT INT TERM
  set +e

  if (( container_intent )); then
    remove_container "$container_name" || containers_stopped=0
  fi
  if (( probe_intent )); then
    remove_container "$probe_name" || containers_stopped=0
  fi

  if (( containers_stopped )); then
    remove_firewall_family ip6tables "$chain6" "$jump6_intent" "$chain6_intent"
    remove_firewall_family iptables "$chain4" "$jump4_intent" "$chain4_intent"
  elif (( jump4_intent || jump6_intent || chain4_intent || chain6_intent )); then
    echo "retaining the UID egress boundary because a client container may still be running" >&2
  fi

  if [[ -n $expiry_server_pid ]]; then
    kill "$expiry_server_pid" 2>/dev/null || true
    wait "$expiry_server_pid" 2>/dev/null || true
  fi
  if [[ -n $server_pid ]]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi

  if (( interop_spec_intent )); then
    rm -f -- "$interop_spec" || cleanup_failed=1
  fi
  rm -f -- "$cli_bin" "$server_log" "$expiry_server_log" "$expiry_ready" || cleanup_failed=1
  rmdir -- "$run_dir" || cleanup_failed=1

  if (( status == 0 && cleanup_failed != 0 )); then
    status=1
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

wait_until_ready() {
  local url=$1
  local marker=$2
  local log=$3
  for _ in $(seq 1 60); do
    if { [[ -z $marker ]] || [[ -f $marker ]]; } && curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  sed -n '1,200p' "$log" >&2
  echo "Lite server did not become ready" >&2
  return 1
}

rule_packets() {
  local family=$1
  local chain=$2
  local line=$3
  local packets
  packets="$(sudo -n "$family" -w 5 -L "$chain" -n -v -x --line-numbers |
    awk -v line="$line" '$1 == line { print $2 }')"
  if [[ ! $packets =~ ^[0-9]+$ ]]; then
    echo "could not read firewall counter $family/$chain/$line" >&2
    return 1
  fi
  printf '%s\n' "$packets"
}

configure_selected_port() {
  local port=$1
  sudo -n iptables -w 5 -F "$chain4"
  # RETURN preserves every pre-existing host OUTPUT rule after our observation
  # jump; ACCEPT here would accidentally weaken the runner's own firewall.
  sudo -n iptables -w 5 -A "$chain4" -o lo -d 127.0.0.1/32 -p tcp --dport "$port" -j RETURN
  sudo -n iptables -w 5 -A "$chain4" -j REJECT
  sudo -n iptables -w 5 -Z "$chain4"
  sudo -n ip6tables -w 5 -Z "$chain6"
}

audit_phase() {
  local phase=$1
  local port=$2
  local allowed rejected4 rejected6
  allowed="$(rule_packets iptables "$chain4" 1)"
  rejected4="$(rule_packets iptables "$chain4" 2)"
  rejected6="$(rule_packets ip6tables "$chain6" 1)"
  printf '%s egress counters: selected-port-%s=%s rejected-ipv4=%s rejected-ipv6=%s\n' \
    "$phase" "$port" "$allowed" "$rejected4" "$rejected6"
  if (( allowed == 0 )); then
    echo "$phase interoperability did not exercise selected API port $port" >&2
    return 1
  fi
  if (( rejected4 != 0 || rejected6 != 0 )); then
    echo "$phase client process tree attempted unexpected IP egress" >&2
    return 1
  fi
}

run_client_phase() {
  container_intent=1
  set +e
  docker run --name "$container_name" --init --pull=never --network=host --userns=host --ipc=host \
    --user "$client_uid:$client_uid" --cap-drop=ALL --security-opt=no-new-privileges \
    -e HOME=/tmp \
    -e XDG_CACHE_HOME=/tmp/.cache \
    -e CI=1 \
    -e BROWSER_TEST_EXTERNAL_SERVER=1 \
    -e BROWSER_TEST_BASE_URL=http://127.0.0.1:4014 \
    -e BROWSER_TEST_EXPIRY_BASE_URL=http://127.0.0.1:4015 \
    -e BURNERPAD_BIN=/opt/burnerpad \
    -e PLAYWRIGHT_OUTPUT_DIR=/tmp/test-results \
    -e HTTP_PROXY= -e HTTPS_PROXY= -e ALL_PROXY= -e NO_PROXY= \
    -e http_proxy= -e https_proxy= -e all_proxy= -e no_proxy= \
    -v "$repo_root:/work:ro" \
    -v "$cli_bin:/opt/burnerpad:ro" \
    -w /work/burnerpad-lite/test/browser \
    "$playwright_image" \
    /work/burnerpad-lite/test/browser/node_modules/.bin/playwright \
      test cli-interop.spec.mjs --project=chromium "$@"
  phase_status=$?
  set -e
  if ! remove_container "$container_name"; then
    return 1
  fi
  container_intent=0
}

# Finish all setup that may legitimately use the network before installing the
# client UID's deny-by-default boundary. --pull=never below makes this ordering
# executable rather than documentary.
docker pull "$playwright_image"
go build -o "$cli_bin" ./cmd/burnerpad

# Playwright resolves test files within Lite's configured test directory. The
# destination must exist before the repository is mounted read-only: OCI cannot
# create a nested bind-mount target below that read-only parent on a fresh
# checkout. Refuse an upstream collision and remove only the file we install.
if [[ -e $interop_spec || -L $interop_spec ]]; then
  echo "reserved Lite interoperability spec path already exists" >&2
  exit 1
fi
interop_spec_intent=1
cp -- "$repo_root/integration/interop.spec.mjs" "$interop_spec"

(cd burnerpad-lite &&
  PORT=4014 RATE_LIMIT=100000 BAN_THRESHOLD=500000 GLOBAL_CEILING=1000000 MAX_SECRETS=100000 \
    mix run --no-halt) >"$server_log" 2>&1 &
server_pid=$!
wait_until_ready http://127.0.0.1:4014/readyz "" "$server_log"

(cd burnerpad-lite &&
  PORT=4015 RATE_LIMIT=100000 BAN_THRESHOLD=500000 GLOBAL_CEILING=1000000 MAX_SECRETS=100000 \
    BURNERPAD_EXPIRY_TEST=1 BURNERPAD_EXPIRY_READY_FILE="$expiry_ready" \
    mix run --no-compile --no-halt ../integration/lite_expiry_bootstrap.exs) >"$expiry_server_log" 2>&1 &
expiry_server_pid=$!
wait_until_ready http://127.0.0.1:4015/readyz "$expiry_ready" "$expiry_server_log"

chain4_intent=1
sudo -n iptables -w 5 -N "$chain4"
sudo -n iptables -w 5 -A "$chain4" -j REJECT
jump4_intent=1
sudo -n iptables -w 5 -I OUTPUT 1 -m owner --uid-owner "$client_uid" -j "$chain4"

chain6_intent=1
sudo -n ip6tables -w 5 -N "$chain6"
sudo -n ip6tables -w 5 -A "$chain6" -j REJECT
jump6_intent=1
sudo -n ip6tables -w 5 -I OUTPUT 1 -m owner --uid-owner "$client_uid" -j "$chain6"

# Prove both owner-match boundaries reject a numeric loopback destination, then
# zero their counters so only the real client matrix contributes evidence.
probe_intent=1
docker run --name "$probe_name" --pull=never --network=host --userns=host \
  --user "$client_uid:$client_uid" --cap-drop=ALL --security-opt=no-new-privileges \
  "$playwright_image" node -e '
    const net = require("node:net");
    const rejected = (host) => new Promise((resolve, reject) => {
      const socket = net.connect({host, port: 9});
      socket.setTimeout(5000);
      socket.once("connect", () => { socket.destroy(); reject(new Error("unexpected connection")); });
      socket.once("error", () => resolve());
      socket.once("timeout", () => { socket.destroy(); reject(new Error("probe timed out")); });
    });
    (async () => { await rejected("127.0.0.1"); await rejected("::1"); })()
      .catch(() => process.exit(1));
  '
docker rm "$probe_name" >/dev/null
probe_intent=0

probe_rejected4="$(rule_packets iptables "$chain4" 1)"
probe_rejected6="$(rule_packets ip6tables "$chain6" 1)"
if (( probe_rejected4 == 0 || probe_rejected6 == 0 )); then
  echo "egress boundary self-test did not reach both reject rules" >&2
  exit 1
fi
readonly expiry_test_title='Go CLI observes expiry in the real Lite store'

configure_selected_port 4014
run_client_phase --grep-invert "$expiry_test_title"
main_status=$phase_status
audit_phase main 4014
if (( main_status != 0 )); then
  exit "$main_status"
fi

configure_selected_port 4015
run_client_phase --grep "$expiry_test_title"
expiry_status=$phase_status
audit_phase expiry 4015
exit "$expiry_status"
