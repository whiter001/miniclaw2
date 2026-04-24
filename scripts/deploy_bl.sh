#!/usr/bin/env bash

set -euo pipefail

host="${MINICLAW_DEPLOY_HOST:-bl}"
remote_user="${MINICLAW_REMOTE_USER:-root}"
remote_home="${MINICLAW_REMOTE_HOME:-/root}"
remote_repo="${MINICLAW_REMOTE_REPO:-/bl/project/miniclaw/repo}"
remote_config="${MINICLAW_REMOTE_CONFIG:-$remote_home/.config/miniclaw/config}"
remote_service="${MINICLAW_REMOTE_SERVICE:-miniclaw-gateway}"
remote_webhook_port="${MINICLAW_REMOTE_WEBHOOK_PORT:-18080}"
service_memory_high="${MINICLAW_SERVICE_MEMORY_HIGH:-320M}"
service_memory_max="${MINICLAW_SERVICE_MEMORY_MAX:-356M}"
service_cpu_quota="${MINICLAW_SERVICE_CPU_QUOTA:-50%}"
service_tasks_max="${MINICLAW_SERVICE_TASKS_MAX:-64}"
deploy_workspace="${MINICLAW_DEPLOY_WORKSPACE:-$PWD}"
linux_arch="${MINICLAW_LINUX_ARCH:-amd64}"
remote_mcp_config="${MINICLAW_REMOTE_MCP_CONFIG:-$remote_home/.config/miniclaw/mcp.json}"

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

run_ssh() {
    ssh "${remote_user}@${host}" "$@"
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
        -czf - . | run_ssh "set -e; mkdir -p '$remote_repo'; find '$remote_repo' -mindepth 1 -maxdepth 1 -exec rm -rf {} +; cd '$remote_repo'; tar xzf -; chmod +x '$remote_repo/miniclaw' '$remote_repo/build.sh' '$remote_repo/scripts/deploy_bl.sh' || true"
}

install_remote_service_unit() {
    log "installing/updating systemd unit $remote_service"
    run_ssh "set -e
cat > /etc/systemd/system/$remote_service.service <<'EOF'
[Unit]
Description=MiniClaw QQ Gateway
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$remote_repo
ExecStart=$remote_repo/miniclaw gateway --webhook-port $remote_webhook_port
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
sleep 2
journalctl -u '$remote_service' -n 40 --no-pager"
}

verify_remote_webhook() {
    log "verifying remote webhook callback page"
    run_ssh "set -e
curl -fsS 'http://127.0.0.1:$remote_webhook_port/qq-callback?code=smoke' | grep -q 'MiniClaw QQ Callback'"
}

verify_remote_web_search() {
    log "verifying built-in web_search MCP tool"
    run_ssh "set -e
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
remote_api_key=\$(sed -n 's/^api_key=//p' '$remote_config' | head -n 1)
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
    require_cmd ssh
    require_cmd tar
    require_cmd curl

    log "starting deploy to $host"
    build_local_binary
    sync_repo
    install_remote_mmx_cli
    install_remote_mmx_mcp_config
    install_remote_service_unit
    apply_remote_service_limits
    restart_remote_service
    verify_remote_webhook
    verify_remote_web_search
    verify_remote_understand_image
    verify_remote_mmx_search
    log "deploy completed successfully"
}

main "$@"
