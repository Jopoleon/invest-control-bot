#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

if [[ -t 1 ]]; then
    COLOR_RESET=$'\033[0m'
    COLOR_BLUE=$'\033[1;34m'
    COLOR_GREEN=$'\033[1;32m'
    COLOR_YELLOW=$'\033[1;33m'
    COLOR_RED=$'\033[1;31m'
else
    COLOR_RESET=""
    COLOR_BLUE=""
    COLOR_GREEN=""
    COLOR_YELLOW=""
    COLOR_RED=""
fi

info() {
    printf "%s==>%s %s\n" "${COLOR_BLUE}" "${COLOR_RESET}" "$*"
}

success() {
    printf "%s==>%s %s\n" "${COLOR_GREEN}" "${COLOR_RESET}" "$*"
}

warn() {
    printf "%s==>%s %s\n" "${COLOR_YELLOW}" "${COLOR_RESET}" "$*"
}

fail() {
    printf "%s==>%s %s\n" "${COLOR_RED}" "${COLOR_RESET}" "$*" >&2
}

APP_NAME="${APP_NAME:-invest-control-bot}"
BUILD_PACKAGE="${BUILD_PACKAGE:-./cmd/server}"
SSH_HOST="${SSH_HOST:-investcontrol-server}"
REMOTE_APP_DIR="${REMOTE_APP_DIR:-/home/investcontrol/apps/invest-control-bot}"
DEPLOY_LAYOUT="${DEPLOY_LAYOUT:-simple}"
GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"
CGO_ENABLED="${CGO_ENABLED:-0}"
RESTART_CMD="${RESTART_CMD:-}"
REMOTE_SERVICE_NAME="${REMOTE_SERVICE_NAME:-invest-control-bot}"
SKIP_RESTART="${SKIP_RESTART:-0}"
KEEP_RELEASES="${KEEP_RELEASES:-5}"
SHOW_SERVICE_STATUS="${SHOW_SERVICE_STATUS:-0}"
SHOW_SERVICE_LOGS="${SHOW_SERVICE_LOGS:-0}"
SERVICE_LOG_LINES="${SERVICE_LOG_LINES:-50}"

cd "${REPO_ROOT}"

require_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        fail "missing required command: $1"
        exit 1
    fi
}

require_cmd go
require_cmd git
require_cmd ssh
require_cmd scp

mkdir -p "${REPO_ROOT}/.dist"

commit_sha="$(git rev-parse --short HEAD)"
timestamp="$(date +%Y%m%d%H%M%S)"
release_name="${timestamp}-${commit_sha}"
local_artifact="${REPO_ROOT}/.dist/${APP_NAME}-${GOOS}-${GOARCH}"

info "building ${BUILD_PACKAGE} for ${GOOS}/${GOARCH}"
CGO_ENABLED="${CGO_ENABLED}" GOOS="${GOOS}" GOARCH="${GOARCH}" \
    go build -o "${local_artifact}" "${BUILD_PACKAGE}"

case "${DEPLOY_LAYOUT}" in
    simple)
        remote_binary_path="${REMOTE_BINARY_PATH:-${REMOTE_APP_DIR}/current/${APP_NAME}}"
        remote_binary_dir="$(dirname "${remote_binary_path}")"
        remote_upload_path="${REMOTE_UPLOAD_PATH:-${REMOTE_APP_DIR}/.deploy/${APP_NAME}.${commit_sha}.tmp}"
        remote_upload_dir="$(dirname "${remote_upload_path}")"
        info "preparing remote target ${remote_binary_dir}"
        ssh "${SSH_HOST}" "mkdir -p '${remote_binary_dir}' '${remote_upload_dir}'"

        info "uploading binary to temporary path ${SSH_HOST}:${remote_upload_path}"
        scp "${local_artifact}" "${SSH_HOST}:${remote_upload_path}"

        info "activating binary in-place"
        ssh "${SSH_HOST}" "\
            chmod +x '${remote_upload_path}' && \
            mv '${remote_upload_path}' '${remote_binary_path}' && \
            printf '%s\n' '${commit_sha}' > '${remote_binary_dir}/REVISION'"
        ;;
    releases)
        remote_release_dir="${REMOTE_APP_DIR}/releases/${release_name}"
        info "preparing remote release directory ${remote_release_dir}"
        ssh "${SSH_HOST}" "mkdir -p '${REMOTE_APP_DIR}/releases' '${remote_release_dir}'"

        info "uploading binary to ${SSH_HOST}"
        scp "${local_artifact}" "${SSH_HOST}:${remote_release_dir}/${APP_NAME}"

        info "activating release ${release_name}"
        ssh "${SSH_HOST}" "\
            chmod +x '${remote_release_dir}/${APP_NAME}' && \
            printf '%s\n' '${commit_sha}' > '${remote_release_dir}/REVISION' && \
            ln -sfn '${remote_release_dir}' '${REMOTE_APP_DIR}/current'"
        ;;
    *)
        fail "unsupported DEPLOY_LAYOUT=${DEPLOY_LAYOUT}; expected simple or releases"
        exit 1
        ;;
esac

if [[ "${SKIP_RESTART}" == "1" ]]; then
    RESTART_CMD=""
fi

if [[ -z "${RESTART_CMD}" && -n "${REMOTE_SERVICE_NAME}" ]]; then
    RESTART_CMD="sudo systemctl restart ${REMOTE_SERVICE_NAME}"
fi

if [[ -n "${RESTART_CMD}" ]]; then
    info "running remote restart command"
    ssh "${SSH_HOST}" "${RESTART_CMD}"
    if [[ -n "${REMOTE_SERVICE_NAME}" && "${SHOW_SERVICE_STATUS}" == "1" ]]; then
        info "remote service status: ${REMOTE_SERVICE_NAME}"
        ssh "${SSH_HOST}" "sudo systemctl --no-pager --full status '${REMOTE_SERVICE_NAME}' || true"
    fi
    if [[ -n "${REMOTE_SERVICE_NAME}" && "${SHOW_SERVICE_LOGS}" == "1" ]]; then
        info "remote service logs via journalctl: ${REMOTE_SERVICE_NAME}"
        ssh "${SSH_HOST}" "sudo journalctl -u '${REMOTE_SERVICE_NAME}' -n '${SERVICE_LOG_LINES}' --no-pager || true"
    fi
else
    warn "restart skipped; set SKIP_RESTART=0 or provide RESTART_CMD/REMOTE_SERVICE_NAME"
fi

if [[ "${DEPLOY_LAYOUT}" == "releases" && "${KEEP_RELEASES}" =~ ^[0-9]+$ ]] && (( KEEP_RELEASES > 0 )); then
    info "pruning old releases, keeping last ${KEEP_RELEASES}"
    ssh "${SSH_HOST}" "cd '${REMOTE_APP_DIR}/releases' && ls -1dt */ 2>/dev/null | tail -n +$((KEEP_RELEASES + 1)) | xargs -r rm -rf --"
fi

success "done"
if [[ "${DEPLOY_LAYOUT}" == "releases" ]]; then
    success "active release: ${remote_release_dir}"
    success "current symlink: ${REMOTE_APP_DIR}/current"
else
    success "active binary: ${remote_binary_path}"
    success "revision file: ${remote_binary_dir}/REVISION"
fi
if [[ -n "${REMOTE_SERVICE_NAME}" ]]; then
    success "logs: sudo journalctl -u ${REMOTE_SERVICE_NAME} -n ${SERVICE_LOG_LINES} --no-pager"
fi
