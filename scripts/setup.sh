#!/usr/bin/env bash

set -euo pipefail

KRAKEN_HOME="${HOME}/.kraken"
SHELL_NAME=$(basename "${SHELL}")

case "${SHELL_NAME}" in
    bash)
        if [[ "${OSTYPE}" == "darwin"* ]]; then
            RC_FILE="${HOME}/.bash_profile"
        else
            RC_FILE="${HOME}/.bashrc"
        fi

        INIT_FILE="${KRAKEN_HOME}/bashrc"

        INIT_CONTENT=$(cat <<EOF
export PATH="\$PATH:${KRAKEN_HOME}/bin"
source <(kraken completion bash)
EOF
)
        ;;

    zsh)
        RC_FILE="${HOME}/.zshrc"

        INIT_FILE="${KRAKEN_HOME}/zshrc"

        INIT_CONTENT=$(cat <<EOF
export PATH="\$PATH:${KRAKEN_HOME}/bin"
source <(kraken completion zsh)
EOF
)
        ;;

    fish)
        mkdir -p "${HOME}/.config/fish"

        RC_FILE="${HOME}/.config/fish/config.fish"

        INIT_FILE="${KRAKEN_HOME}/fishrc"

        INIT_CONTENT=$(cat <<EOF
set -gx PATH \$PATH ${KRAKEN_HOME}/bin
kraken completion fish | source
EOF
)
        ;;

    *)
        echo "Error: this terminal (${SHELL_NAME}) is not supported for automatic installation." >&2
        echo "Only bash, zsh, and fish are supported." >&2
        exit 1
        ;;
esac

mkdir -p "${KRAKEN_HOME}"

if [ ! -d "${KRAKEN_HOME}/bin" ]; then
    echo "Installing kraken..."
    curl -fsSL https://raw.githubusercontent.com/kraken-hub/kraken/main/scripts/install.sh |
        bash -s -- --dir "${KRAKEN_HOME}/bin"
fi

ENV_FILE="${KRAKEN_HOME}/env"

if [ ! -f "${ENV_FILE}" ]; then
    cat > "${ENV_FILE}" <<EOF
KRAKEN_HOME=${KRAKEN_HOME}
#KRAKEN_SHELL=/bin/bash
#KRAKEN_NO_BANNER=true
#NO_COLOR=true
EOF
fi

printf '%s\n' "${INIT_CONTENT}" > "${INIT_FILE}"
echo "Generated ${INIT_FILE}"

append_if_missing() {
    local line="$1"
    local file="$2"

    if grep -qF "$line" "$file" 2>/dev/null; then
        echo "Line already present in ${file}, skipping: ${line}"
    else
        printf '\n%s\n' "$line" >> "$file"
        echo "Added to ${file}: ${line}"
    fi
}

SOURCE_LINE="source ${INIT_FILE}"
append_if_missing "${SOURCE_LINE}" "${RC_FILE}"

echo "Restart your terminal or run: source ${INIT_FILE}"
