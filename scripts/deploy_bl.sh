#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
deploy_mode="${MINICLAW_DEPLOY_MODE:-podman-alpine}"

case "$deploy_mode" in
    podman-alpine|podman_alpine|podman|remote-podman|remote-podman-alpine)
        deploy_mode="remote-podman-alpine"
        ;;
    local-podman|local-podman-alpine)
        exec bash "$repo_root/scripts/deploy_podman_alpine.sh" "$@"
        ;;
    remote-systemd|systemd|ssh-systemd)
        deploy_mode="remote-systemd"
        ;;
    *)
        printf '[deploy] error: unsupported MINICLAW_DEPLOY_MODE: %s\n' "$deploy_mode" >&2
        exit 1
        ;;
esac

host="${MINICLAW_DEPLOY_HOST:-bl}"
remote_user="${MINICLAW_REMOTE_USER:-root}"
remote_home="${MINICLAW_REMOTE_HOME:-/root}"
remote_repo="${MINICLAW_REMOTE_REPO:-/bl/project/miniclaw/repo}"
remote_config="${MINICLAW_REMOTE_CONFIG:-$remote_home/.config/miniclaw/config}"
remote_service="${MINICLAW_REMOTE_SERVICE:-miniclaw-gateway}"
remote_channel="${MINICLAW_GATEWAY_CHANNEL:-qq}"
remote_env_file="${MINICLAW_REMOTE_ENV_FILE:-/etc/miniclaw/miniclaw.env}"
remote_app_home="${MINICLAW_REMOTE_APP_HOME:-$remote_home/.miniclaw}"
remote_workspace="${MINICLAW_REMOTE_WORKSPACE:-$remote_app_home/workspace}"
remote_webhook_port="${MINICLAW_REMOTE_WEBHOOK_PORT:-18080}"
remote_weixin_login="${MINICLAW_REMOTE_WEIXIN_LOGIN:-0}"
remote_weixin_login_timeout="${MINICLAW_REMOTE_WEIXIN_LOGIN_TIMEOUT:-8m}"
service_memory_high="${MINICLAW_SERVICE_MEMORY_HIGH:-320M}"
service_memory_max="${MINICLAW_SERVICE_MEMORY_MAX:-356M}"
service_cpu_quota="${MINICLAW_SERVICE_CPU_QUOTA:-50%}"
service_tasks_max="${MINICLAW_SERVICE_TASKS_MAX:-64}"
deploy_workspace="${MINICLAW_DEPLOY_WORKSPACE:-$PWD}"
linux_arch="${MINICLAW_LINUX_ARCH:-amd64}"
remote_mcp_config="${MINICLAW_REMOTE_MCP_CONFIG:-$remote_home/.config/miniclaw/mcp.json}"
ssh_keepalive_interval="${MINICLAW_SSH_KEEPALIVE_INTERVAL:-30}"
ssh_keepalive_count_max="${MINICLAW_SSH_KEEPALIVE_COUNT_MAX:-20}"
remote_podman_image="${MINICLAW_PODMAN_IMAGE:-miniclaw:alpine}"
remote_podman_container="${MINICLAW_PODMAN_CONTAINER:-$remote_service}"
remote_podman_state_root="${MINICLAW_PODMAN_STATE_ROOT:-$remote_app_home}"
remote_podman_env_file="${MINICLAW_PODMAN_ENV_FILE:-$remote_env_file}"
remote_podman_home="${MINICLAW_PODMAN_HOME:-$remote_app_home}"
remote_podman_config="${MINICLAW_PODMAN_CONFIG:-$remote_config}"
remote_podman_mcp_config="${MINICLAW_PODMAN_MCP_CONFIG:-$remote_mcp_config}"
remote_podman_qq_port="${MINICLAW_PODMAN_QQ_PORT:-$remote_webhook_port}"
remote_podman_qq_container_port="${MINICLAW_PODMAN_QQ_CONTAINER_PORT:-$remote_webhook_port}"
remote_podman_weixin_login="${MINICLAW_PODMAN_WEIXIN_LOGIN:-$remote_weixin_login}"
remote_podman_weixin_login_timeout="${MINICLAW_PODMAN_WEIXIN_LOGIN_TIMEOUT:-$remote_weixin_login_timeout}"

log() {
    printf '[deploy] %s\n' "$*"
}

fail() {
    printf '[deploy] error: %s\n' "$*" >&2
    exit 1
}

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || fail "missing command: $1"
}

channel_label() {
    case "$1" in
        qq) printf 'QQ' ;;
        weixin) printf 'Weixin' ;;
        *) printf '%s' "$1" ;;
    esac
}

ensure_supported_channel() {
    case "$remote_channel" in
        qq|weixin) ;;
        *) fail "unsupported gateway channel: $remote_channel (expected qq or weixin)" ;;
    esac
}

remote_gateway_exec() {
    local cmd
    cmd="$remote_repo/miniclaw gateway --channel $remote_channel"
    if [ "$remote_channel" = "qq" ]; then
        cmd="$cmd --webhook-port $remote_webhook_port"
    fi
    printf '%s' "$cmd"
}

run_ssh() {
    ssh \
        -o ServerAliveInterval="$ssh_keepalive_interval" \
        -o ServerAliveCountMax="$ssh_keepalive_count_max" \
        "${remote_user}@${host}" "$@"
}

warn_legacy_service_conflict() {
    if [ "$remote_service" = "miniclaw-gateway" ] || [ "$remote_channel" != "qq" ]; then
        return
    fi
    legacy_details="$(run_ssh "set -e
if systemctl list-unit-files | grep -q '^miniclaw-gateway\\.service'; then
    if systemctl is-enabled miniclaw-gateway >/dev/null 2>&1 || systemctl is-active miniclaw-gateway >/dev/null 2>&1; then
        systemctl show miniclaw-gateway -p ActiveState -p UnitFileState -p SubState
        systemctl cat miniclaw-gateway | sed -n '1,80p'
    fi
fi" || true)"
    if [ -z "$legacy_details" ]; then
        return
    fi
    printf '[deploy] warning: detected legacy miniclaw-gateway service while deploying %s.\n' "$remote_service" >&2
    printf '[deploy] warning: legacy qq service may still bind port %s and block %s.\n' "$remote_webhook_port" "$remote_service" >&2
    printf '[deploy] warning: disable the old unit first: systemctl disable --now miniclaw-gateway\n' >&2
    printf '%s\n' "$legacy_details" >&2
}

build_local_binary() {
    log "building local linux binary"
    cd "$deploy_workspace"
    CGO_ENABLED=0 GOOS=linux GOARCH="$linux_arch" go build -trimpath -ldflags='-s -w' -o miniclaw ./cmd/miniclaw
}

sync_repo() {
    log "syncing repository to $host:$remote_repo"
    cd "$deploy_workspace"
    COPYFILE_DISABLE=1 tar \
        --exclude='.git' \
        --exclude='node_modules' \
        --exclude='.env' \
        --exclude='miniclaw.exe' \
        --exclude='coverage.out' \
        --exclude='*.test' \
        --exclude='._*' \
        -czf - . | run_ssh "set -e; mkdir -p '$remote_repo'; find '$remote_repo' -mindepth 1 -maxdepth 1 -exec rm -rf {} +; cd '$remote_repo'; tar xzf -; chmod +x '$remote_repo/miniclaw' '$remote_repo/build.sh' '$remote_repo/scripts/deploy_bl.sh' '$remote_repo/scripts/deploy_podman_alpine.sh' '$remote_repo/scripts/deploy_bl_dual.sh' || true"
}

ensure_remote_podman_mcp_config() {
    if [ -z "$remote_podman_mcp_config" ]; then
        return
    fi
    log "ensuring remote Podman MCP config at $remote_podman_mcp_config"
    run_ssh "set -e
target='$remote_podman_mcp_config'
target_dir=\$(dirname \"\$target\")
mkdir -p \"\$target_dir\"
if [ ! -f \"\$target\" ]; then
    cp '$remote_repo/examples/mcp.mmx.json.example' \"\$target\"
    printf '[deploy] created MCP config: %s\n' \"\$target\"
else
    printf '[deploy] keeping existing MCP config: %s\n' \"\$target\"
fi"
}

run_remote_podman_deploy() {
    log "running remote Podman Alpine deployment on $host"
    run_ssh "set -e
command -v podman >/dev/null 2>&1
cd '$remote_repo'
export MINICLAW_DEPLOY_WORKSPACE='$remote_repo'
export MINICLAW_SKIP_BUILD=1
export MINICLAW_GATEWAY_CHANNEL='$remote_channel'
export MINICLAW_PODMAN_IMAGE='$remote_podman_image'
export MINICLAW_PODMAN_CONTAINER='$remote_podman_container'
export MINICLAW_PODMAN_STATE_ROOT='$remote_podman_state_root'
export MINICLAW_PODMAN_ENV_FILE='$remote_podman_env_file'
export MINICLAW_PODMAN_HOME='$remote_podman_home'
export MINICLAW_PODMAN_CONFIG='$remote_podman_config'
export MINICLAW_PODMAN_MCP_CONFIG='$remote_podman_mcp_config'
export MINICLAW_PODMAN_QQ_PORT='$remote_podman_qq_port'
export MINICLAW_PODMAN_QQ_CONTAINER_PORT='$remote_podman_qq_container_port'
export MINICLAW_PODMAN_WEIXIN_LOGIN='$remote_podman_weixin_login'
export MINICLAW_PODMAN_WEIXIN_LOGIN_TIMEOUT='$remote_podman_weixin_login_timeout'
bash './scripts/deploy_podman_alpine.sh'"
}

install_remote_env_template() {
    log "ensuring remote env file at $remote_env_file"
    run_ssh "set -e
remote_env_file='$remote_env_file'
remote_env_dir=\$(dirname \"\$remote_env_file\")
mkdir -p \"\$remote_env_dir\"
if [ ! -f \"\$remote_env_file\" ]; then
    cp '$remote_repo/.env.example' \"\$remote_env_file\"
    sed -i.bak \
        -e 's|^MINICLAW_HOME=.*|MINICLAW_HOME=$remote_app_home|' \
        -e 's|^MINICLAW_WORKSPACE=.*|MINICLAW_WORKSPACE=$remote_workspace|' \
        -e 's|^MINICLAW_MCP_CONFIG_PATH=.*|MINICLAW_MCP_CONFIG_PATH=$remote_mcp_config|' \
        -e 's|^MINICLAW_GATEWAY_CHANNEL=.*|MINICLAW_GATEWAY_CHANNEL=$remote_channel|' \
        -e 's|^MINICLAW_QQ_WEBHOOK_PORT=.*|MINICLAW_QQ_WEBHOOK_PORT=$remote_webhook_port|' \
        \"\$remote_env_file\"
    rm -f \"\$remote_env_file.bak\"
    printf '[deploy] created env template: %s\n' \"\$remote_env_file\"
else
    printf '[deploy] keeping existing env file: %s\n' \"\$remote_env_file\"
fi
chmod 600 \"\$remote_env_file\" || true"
}

run_remote_onboard() {
    log "running remote onboard"
    run_ssh "set -e
export HOME='$remote_home'
if [ -f '$remote_env_file' ]; then
    set -a
    . '$remote_env_file'
    set +a
fi
'$remote_repo/miniclaw' onboard"
}

ensure_remote_weixin_ready() {
    if [ "$remote_channel" != "weixin" ]; then
        return
    fi
    log "checking remote weixin account state"
    run_ssh "set -e
export HOME='$remote_home'
if [ -f '$remote_env_file' ]; then
    set -a
    . '$remote_env_file'
    set +a
fi
status=\$('$remote_repo/miniclaw' status)
printf '%s\n' \"\$status\"
if printf '%s\n' \"\$status\" | grep -q 'weixin configured: true'; then
    exit 0
fi
if [ '$remote_weixin_login' = '1' ]; then
    '$remote_repo/miniclaw' gateway login --channel weixin --timeout '$remote_weixin_login_timeout'
    '$remote_repo/miniclaw' gateway accounts --channel weixin
    exit 0
fi
printf '[deploy] error: remote weixin account is not configured. Set MINICLAW_WEIXIN_TOKEN or rerun with MINICLAW_REMOTE_WEIXIN_LOGIN=1.\n' >&2
exit 1"
}

install_remote_service_unit() {
    log "installing/updating systemd unit $remote_service"
    local service_description
    local gateway_exec
    service_description="MiniClaw $(channel_label "$remote_channel") Gateway"
    gateway_exec="$(remote_gateway_exec)"
    run_ssh "set -e
cat > /etc/systemd/system/$remote_service.service <<'EOF'
[Unit]
Description=$service_description
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=$remote_repo
EnvironmentFile=-$remote_env_file
ExecStart=$gateway_exec
Restart=always
RestartSec=3
Environment=HOME=$remote_home

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable $remote_service >/dev/null 2>&1 || true
systemctl cat $remote_service"
}

install_remote_mmx_cli() {
    log "installing/updating mmx-cli on remote host"
    run_ssh "set -e
command -v npm >/dev/null 2>&1
npm install -g mmx-cli >/tmp/miniclaw-mmx-install.log 2>&1
mmx --version
tail -n 20 /tmp/miniclaw-mmx-install.log || true"
}

install_remote_mmx_mcp_config() {
    log "installing mmx MCP command config to $remote_mcp_config"
    run_ssh "set -e
remote_mcp_config='$remote_mcp_config'
remote_mcp_dir=\$(dirname \"\$remote_mcp_config\")
mkdir -p \"\$remote_mcp_dir\"
if [ -f \"\$remote_mcp_config\" ]; then
    backup_ts=\$(date +%Y%m%d%H%M%S)
    cp \"\$remote_mcp_config\" \"\$remote_mcp_config.bak.\$backup_ts\"
fi
cp '$remote_repo/examples/mcp.mmx.json.example' \"\$remote_mcp_config\"
sed -n '1,220p' \"\$remote_mcp_config\""
}

apply_remote_service_limits() {
    log "applying systemd resource limits for $remote_service"
    run_ssh "set -e
override_dir='/etc/systemd/system/$remote_service.service.d'
mkdir -p \"\$override_dir\"
cat > \"\$override_dir/override.conf\" <<'EOF'
[Service]
MemoryHigh=$service_memory_high
MemoryMax=$service_memory_max
CPUQuota=$service_cpu_quota
TasksMax=$service_tasks_max
EOF
systemctl daemon-reload
systemctl show '$remote_service' -p MemoryHigh -p MemoryMax -p CPUQuotaPerSecUSec -p TasksMax"
}

restart_remote_service() {
    log "restarting $remote_service"
    run_ssh "set -e
systemctl restart '$remote_service'
systemctl is-active '$remote_service'
journalctl -u '$remote_service' -n 40 --no-pager"
}

verify_remote_webhook() {
    if [ "$remote_channel" != "qq" ]; then
        return
    fi
    log "verifying remote webhook callback page"
    run_ssh "set -e
curl -fsS 'http://127.0.0.1:$remote_webhook_port/qq-callback?code=smoke' | grep -q 'MiniClaw QQ Callback'"
}

verify_remote_gateway_bootstrap() {
    log "verifying remote $(channel_label "$remote_channel") bootstrap"
    run_ssh "set -e
export HOME='$remote_home'
if [ -f '$remote_env_file' ]; then
    set -a
    . '$remote_env_file'
    set +a
fi
$(remote_gateway_exec) --once"
}

verify_remote_web_search() {
    log "verifying built-in web_search MCP tool"
    run_ssh "set -e
export HOME='$remote_home'
if [ -f '$remote_env_file' ]; then
    set -a
    . '$remote_env_file'
    set +a
fi
rm -rf '$remote_repo/sessions'
mkdir -p '$remote_repo/sessions'
timeout 180s '$remote_repo/miniclaw' agent --workspace '$remote_repo' --mcp -p '请务必使用 web_search 工具搜索 MiniMax MCP，并简短说明搜索结论。' > /tmp/miniclaw-go-websearch.out 2>&1
        latest=\$(find '$remote_repo/sessions' -maxdepth 1 -type f | sort | tail -n 1)
test -n \"\$latest\"
pattern=\$(printf 'tool_name\042:\042web_search')
grep -Fq \"\$pattern\" \"\$latest\"
test -s /tmp/miniclaw-go-websearch.out
sed -n '1,40p' /tmp/miniclaw-go-websearch.out"
}

verify_remote_understand_image() {
    log "verifying built-in understand_image MCP tool"
    run_ssh "set -e
export HOME='$remote_home'
if [ -f '$remote_env_file' ]; then
    set -a
    . '$remote_env_file'
    set +a
fi
rm -rf '$remote_repo/sessions'
mkdir -p '$remote_repo/sessions'
timeout 180s '$remote_repo/miniclaw' agent --workspace '$remote_repo' --mcp -p '请务必使用 understand_image 工具分析这张图片：https://httpbin.org/image/png ，并简短描述图片可用性。' > /tmp/miniclaw-go-image.out 2>&1
        latest=\$(find '$remote_repo/sessions' -maxdepth 1 -type f | sort | tail -n 1)
test -n \"\$latest\"
pattern=\$(printf 'tool_name\042:\042understand_image')
grep -Fq \"\$pattern\" \"\$latest\"
test -s /tmp/miniclaw-go-image.out
sed -n '1,40p' /tmp/miniclaw-go-image.out"
}

verify_remote_mmx_search() {
    log "verifying mmx_search command tool with remote MINICLAW_API_KEY"
    run_ssh "set -e
export HOME='$remote_home'
if [ -f '$remote_env_file' ]; then
    set -a
    . '$remote_env_file'
    set +a
fi
remote_api_key=''
if [ -f '$remote_env_file' ]; then
    remote_api_key=\$(sed -n 's/^MINICLAW_API_KEY=//p' '$remote_env_file' | head -n 1)
fi
if [ -z \"\$remote_api_key\" ] && [ -f '$remote_config' ]; then
    remote_api_key=\$(sed -n 's/^api_key=//p' '$remote_config' | head -n 1)
fi
test -n \"\$remote_api_key\"
rm -rf '$remote_repo/sessions'
mkdir -p '$remote_repo/sessions'
MINICLAW_API_KEY=\"\$remote_api_key\" timeout 180s '$remote_repo/miniclaw' agent --workspace '$remote_repo' --mcp -p '请务必使用 mmx_search 工具搜索 MiniMax MCP，并只返回一条最相关结果的标题和链接。' > /tmp/miniclaw-go-mmx-search.out 2>&1
latest=\$(find '$remote_repo/sessions' -maxdepth 1 -type f | sort | tail -n 1)
test -n \"\$latest\"
pattern=\$(printf 'tool_name\042:\042mmx_search')
grep -Fq \"\$pattern\" \"\$latest\"
test -s /tmp/miniclaw-go-mmx-search.out
sed -n '1,80p' /tmp/miniclaw-go-mmx-search.out"
}

main() {
    ensure_supported_channel
    require_cmd ssh
    require_cmd tar
    require_cmd curl
    require_cmd go

    log "starting deploy to $host (channel=$remote_channel, mode=$deploy_mode)"
    build_local_binary
    sync_repo

    if [ "$deploy_mode" = 'remote-podman-alpine' ]; then
        warn_legacy_service_conflict
        ensure_remote_podman_mcp_config
        run_remote_podman_deploy
        log "deploy completed successfully"
        return
    fi

    warn_legacy_service_conflict
    install_remote_env_template
    run_remote_onboard
    ensure_remote_weixin_ready
    install_remote_mmx_cli
    install_remote_mmx_mcp_config
    install_remote_service_unit
    apply_remote_service_limits
    restart_remote_service
    verify_remote_gateway_bootstrap
    verify_remote_webhook
    verify_remote_web_search
    verify_remote_understand_image
    verify_remote_mmx_search
    log "deploy completed successfully"
}

main "$@"
