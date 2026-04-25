#!/usr/bin/env bash

set -euo pipefail

deploy_workspace="${MINICLAW_DEPLOY_WORKSPACE:-$PWD}"
podman_bin="${MINICLAW_PODMAN_BIN:-podman}"
containerfile_path="${MINICLAW_PODMAN_CONTAINERFILE:-$deploy_workspace/Containerfile.alpine}"
remote_channel="${MINICLAW_GATEWAY_CHANNEL:-weixin}"
linux_arch="${MINICLAW_LINUX_ARCH:-}"
skip_build="${MINICLAW_SKIP_BUILD:-0}"
podman_image="${MINICLAW_PODMAN_IMAGE:-miniclaw:alpine}"
podman_container="${MINICLAW_PODMAN_CONTAINER:-miniclaw-$remote_channel}"
podman_state_root="${MINICLAW_PODMAN_STATE_ROOT:-$deploy_workspace/.podman/$podman_container}"
podman_env_file="${MINICLAW_PODMAN_ENV_FILE:-$podman_state_root/miniclaw.env}"
podman_home="${MINICLAW_PODMAN_HOME:-$podman_state_root/home}"
podman_config="${MINICLAW_PODMAN_CONFIG:-}"
podman_mcp_config="${MINICLAW_PODMAN_MCP_CONFIG:-}"
podman_qq_port="${MINICLAW_PODMAN_QQ_PORT:-18080}"
podman_qq_container_port="${MINICLAW_PODMAN_QQ_CONTAINER_PORT:-18080}"
podman_weixin_login="${MINICLAW_PODMAN_WEIXIN_LOGIN:-0}"
podman_weixin_login_timeout="${MINICLAW_PODMAN_WEIXIN_LOGIN_TIMEOUT:-8m}"

container_home="/var/lib/miniclaw"
container_mcp_config="/etc/miniclaw/mcp.json"

podman_common_args=()

log() {
    printf '[podman-deploy] %s\n' "$*"
}

fail() {
    printf '[podman-deploy] error: %s\n' "$*" >&2
    exit 1
}

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || fail "missing command: $1"
}

ensure_supported_channel() {
    case "$remote_channel" in
        qq|weixin) ;;
        *) fail "unsupported gateway channel: $remote_channel (expected qq or weixin)" ;;
    esac
}

remote_gateway_exec() {
    local -a cmd
    cmd=(gateway --channel "$remote_channel")
    if [ "$remote_channel" = "qq" ]; then
        cmd+=(--webhook-port "$podman_qq_container_port")
    fi
    printf '%s\n' "${cmd[*]}"
}

build_local_binary() {
    if [ "$skip_build" = '1' ]; then
        [ -x "$deploy_workspace/miniclaw" ] || fail "MINICLAW_SKIP_BUILD=1 requires an existing executable at $deploy_workspace/miniclaw"
        log "reusing existing binary $deploy_workspace/miniclaw"
        return
    fi
    if [ -z "$linux_arch" ]; then
        linux_arch="$(go env GOARCH)"
    fi
    log "building local linux binary"
    cd "$deploy_workspace"
    CGO_ENABLED=0 GOOS=linux GOARCH="$linux_arch" go build -trimpath -ldflags='-s -w' -o miniclaw ./cmd/miniclaw
}

build_podman_image() {
    log "building Podman Alpine image $podman_image"
    cd "$deploy_workspace"
    "$podman_bin" build -f "$containerfile_path" -t "$podman_image" .
}

ensure_local_state() {
    log "ensuring local Podman state at $podman_state_root"
    mkdir -p "$podman_state_root" "$podman_home" "$podman_home/.config/miniclaw" "$(dirname "$podman_env_file")"
    if [ ! -f "$podman_env_file" ]; then
        cp "$deploy_workspace/.env.example" "$podman_env_file"
        sed -i.bak \
            -e 's|^MINICLAW_HOME=.*|MINICLAW_HOME=/var/lib/miniclaw|' \
            -e 's|^MINICLAW_WORKSPACE=.*|MINICLAW_WORKSPACE=/var/lib/miniclaw/workspace|' \
            -e 's|^MINICLAW_MCP_CONFIG_PATH=.*|MINICLAW_MCP_CONFIG_PATH=/etc/miniclaw/mcp.json|' \
            -e 's|^MINICLAW_GATEWAY_CHANNEL=.*|MINICLAW_GATEWAY_CHANNEL='$remote_channel'|' \
            -e 's|^MINICLAW_QQ_WEBHOOK_HOST=.*|MINICLAW_QQ_WEBHOOK_HOST=0.0.0.0|' \
            -e 's|^MINICLAW_QQ_WEBHOOK_PORT=.*|MINICLAW_QQ_WEBHOOK_PORT='$podman_qq_container_port'|' \
            "$podman_env_file"
        rm -f "$podman_env_file.bak"
        printf '[podman-deploy] created env template: %s\n' "$podman_env_file"
    else
        printf '[podman-deploy] keeping existing env file: %s\n' "$podman_env_file"
    fi
    if [ -z "$podman_config" ] && [ -f "$HOME/.config/miniclaw/config" ]; then
        podman_config="$HOME/.config/miniclaw/config"
    fi
    if [ -n "$podman_config" ]; then
        [ -f "$podman_config" ] || fail "missing config: $podman_config"
        cp "$podman_config" "$podman_home/.config/miniclaw/config"
        chmod 600 "$podman_home/.config/miniclaw/config" || true
        printf '[podman-deploy] synced config: %s\n' "$podman_home/.config/miniclaw/config"
    fi
}

build_podman_common_args() {
    podman_common_args=(
        --env-file "$podman_env_file"
        -e "HOME=$container_home"
        -e "MINICLAW_HOME=$container_home"
        -e "MINICLAW_WORKSPACE=$container_home/workspace"
        -e "MINICLAW_GATEWAY_CHANNEL=$remote_channel"
        -e "MINICLAW_QQ_WEBHOOK_HOST=0.0.0.0"
        -e "MINICLAW_QQ_WEBHOOK_PORT=$podman_qq_container_port"
        -e "MINICLAW_MCP_CONFIG_PATH=$container_mcp_config"
        -v "$podman_home:$container_home"
    )
    if [ -n "$podman_mcp_config" ]; then
        [ -f "$podman_mcp_config" ] || fail "missing MCP config: $podman_mcp_config"
        podman_common_args+=(-v "$podman_mcp_config:$container_mcp_config:ro")
    fi
}

run_onboard() {
    log "running container onboard"
    "$podman_bin" run --rm "${podman_common_args[@]}" "$podman_image" onboard
}

ensure_weixin_ready() {
    if [ "$remote_channel" != "weixin" ]; then
        return
    fi
    log "checking Podman Weixin account state"
    status_output="$("$podman_bin" run --rm "${podman_common_args[@]}" "$podman_image" status)"
    printf '%s\n' "$status_output"
    if printf '%s\n' "$status_output" | grep -q 'weixin configured: true'; then
        return
    fi
    if [ "$podman_weixin_login" = '1' ]; then
        "$podman_bin" run --rm "${podman_common_args[@]}" "$podman_image" gateway login --channel weixin --timeout "$podman_weixin_login_timeout"
        "$podman_bin" run --rm "${podman_common_args[@]}" "$podman_image" gateway accounts --channel weixin
        return
    fi
    fail 'weixin account is not configured. Set MINICLAW_WEIXIN_TOKEN in the env file or rerun with MINICLAW_PODMAN_WEIXIN_LOGIN=1'
}

remove_existing_container() {
    if "$podman_bin" container exists "$podman_container"; then
        log "removing existing container $podman_container"
        "$podman_bin" rm -f "$podman_container" >/dev/null
    fi
}

run_container() {
    local -a args
    args=(run -d --name "$podman_container" --restart unless-stopped)
    args+=("${podman_common_args[@]}")
    if [ "$remote_channel" = "qq" ]; then
        args+=(-p "$podman_qq_port:$podman_qq_container_port")
    fi
    args+=("$podman_image" gateway --channel "$remote_channel")
    if [ "$remote_channel" = "qq" ]; then
        args+=(--webhook-port "$podman_qq_container_port")
    fi
    log "starting container $podman_container"
    "$podman_bin" "${args[@]}"
}

verify_container_running() {
    log "verifying container runtime state"
    running="$($podman_bin inspect -f '{{.State.Running}}' "$podman_container")"
    [ "$running" = 'true' ] || fail "container is not running: $podman_container"
    "$podman_bin" logs --tail 20 "$podman_container"
}

verify_gateway_bootstrap() {
    log "verifying container bootstrap"
    local -a args
    args=(run --rm)
    args+=("${podman_common_args[@]}")
    args+=("$podman_image" gateway --channel "$remote_channel" --once)
    if [ "$remote_channel" = "qq" ]; then
        args+=(--webhook-port "$podman_qq_container_port")
    fi
    "$podman_bin" "${args[@]}"
}

verify_qq_webhook() {
    if [ "$remote_channel" != "qq" ]; then
        return
    fi
    log "verifying QQ callback endpoint"
    curl -fsS --retry 5 --retry-delay 1 --retry-connrefused \
        "http://127.0.0.1:$podman_qq_port/qq-callback?code=smoke" | grep -q 'MiniClaw QQ Callback'
}

main() {
    ensure_supported_channel
    if [ "$skip_build" != '1' ]; then
        require_cmd go
    fi
    require_cmd "$podman_bin"
    require_cmd curl

    log "starting Podman Alpine deployment (channel=$remote_channel)"
    build_local_binary
    build_podman_image
    ensure_local_state
    build_podman_common_args
    run_onboard
    ensure_weixin_ready
    remove_existing_container
    run_container
    verify_container_running
    verify_gateway_bootstrap
    verify_qq_webhook
    log "Podman Alpine deployment completed successfully"
}

main "$@"