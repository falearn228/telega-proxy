#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
binary="$repo_dir/bin/tg-proxy-service"
unit="$repo_dir/systemd/tg-proxy-service.service"
env_dir="$HOME/.config/tg-proxy-service"
env_file="$env_dir/proxy.env"

if [[ ! -x "$binary" ]]; then
    printf 'Build the service first: make build-service\n' >&2
    exit 1
fi

install -Dm755 "$binary" "$HOME/.local/bin/tg-proxy-service"
install -Dm644 "$unit" "$HOME/.config/systemd/user/tg-proxy-service.service"

proxy_value=${HTTPS_PROXY:-${https_proxy:-}}
no_proxy_value=${NO_PROXY:-${no_proxy:-}}
if [[ ! -e "$env_file" && -n "$proxy_value" ]]; then
    mkdir -p "$env_dir"
    for value in "$proxy_value" "$no_proxy_value"; do
        if [[ "$value" == *$'\n'* || "$value" == *$'\r'* ]]; then
            printf 'Proxy environment contains a newline\n' >&2
            exit 1
        fi
    done
    proxy_value=${proxy_value//\\/\\\\}
    proxy_value=${proxy_value//\"/\\\"}
    no_proxy_value=${no_proxy_value//\\/\\\\}
    no_proxy_value=${no_proxy_value//\"/\\\"}
    (
        umask 077
        printf 'HTTPS_PROXY="%s"\nNO_PROXY="%s"\n' "$proxy_value" "$no_proxy_value" > "$env_file"
    )
fi
if [[ -e "$env_file" ]]; then
    chmod 600 "$env_file"
fi

systemctl --user daemon-reload
systemctl --user enable --now tg-proxy-service.service
systemctl --user restart tg-proxy-service.service
systemctl --user --no-pager status tg-proxy-service.service
