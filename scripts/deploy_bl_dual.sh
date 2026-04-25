#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
qq_env_default="$repo_root/.deploy.podman.qq.env"
weixin_env_default="$repo_root/.deploy.podman.weixin.env"
qq_env_fallback="$repo_root/examples/deploy.podman.alpine.qq.env.example"
weixin_env_fallback="$repo_root/examples/deploy.podman.alpine.weixin.env.example"
qq_env_file="${MINICLAW_DUAL_QQ_ENV_FILE:-}"
weixin_env_file="${MINICLAW_DUAL_WEIXIN_ENV_FILE:-}"

log() {
    printf '[dual-deploy] %s\n' "$*"
}

fail() {
    printf '[dual-deploy] error: %s\n' "$*" >&2
    exit 1
}

resolve_env_file() {
    local explicit_path="$1"
    local primary_path="$2"
    local fallback_path="$3"
    if [ -n "$explicit_path" ]; then
        [ -f "$explicit_path" ] || fail "env file not found: $explicit_path"
        printf '%s\n' "$explicit_path"
        return
    fi
    if [ -f "$primary_path" ]; then
        printf '%s\n' "$primary_path"
        return
    fi
    if [ -f "$fallback_path" ]; then
        printf '%s\n' "$fallback_path"
        return
    fi
    fail "env file not found: $primary_path (fallback: $fallback_path)"
}

run_deploy() {
    local channel_label="$1"
    local env_file="$2"
    log "deploying $channel_label using $env_file"
    if (
        set -a
        . "$env_file"
        set +a
        cd "$repo_root"
        bash ./scripts/deploy_bl.sh
    ); then
        log "$channel_label deployment succeeded"
        return 0
    fi
    log "$channel_label deployment failed"
    return 1
}

main() {
    local failures=0
    qq_env_file="$(resolve_env_file "$qq_env_file" "$qq_env_default" "$qq_env_fallback")"
    weixin_env_file="$(resolve_env_file "$weixin_env_file" "$weixin_env_default" "$weixin_env_fallback")"

    log "repo root: $repo_root"
    run_deploy qq "$qq_env_file" || failures=$((failures + 1))
    run_deploy weixin "$weixin_env_file" || failures=$((failures + 1))
    if [ "$failures" -ne 0 ]; then
        fail "$failures deployment(s) failed"
    fi
    log "dual deployment completed"
}

main "$@"