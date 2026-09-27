#!/usr/bin/env bash

set -euo pipefail

INSTALL_DIR=/usr/local/bin
BINARY_NAME=kraken
VERSION=latest

GITHUB_RELEASES_URL="https://github.com/kraken-hub/kraken/releases"
GITHUB_RELEASES_API_URL="https://api.github.com/repos/kraken-hub/kraken/releases"

while [ $# -gt 0 ]; do
    case "$1" in
        --dir=*)
            INSTALL_DIR="${1#*=}"
            ;;
        --dir)
            if [ $# -lt 2 ]; then
                echo "Missing value for --dir" >&2
                exit 1
            fi
            INSTALL_DIR="$2"
            shift
            ;;
        --name=*)
            BINARY_NAME="${1#*=}"
            ;;
        --name)
            if [ $# -lt 2 ]; then
                echo "Missing value for --name" >&2
                exit 1
            fi
            BINARY_NAME="$2"
            shift
            ;;
        --version=*)
            VERSION="${1#*=}"
            ;;
        --version)
            if [ $# -lt 2 ]; then
                echo "Missing value for --version" >&2
                exit 1
            fi
            VERSION="$2"
            shift
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac

    shift
done

# Detect OS
case "${OSTYPE}" in
    darwin*)
        OS=darwin
        ;;
    linux*)
        OS=linux
        ;;
    *)
        echo "Unsupported OS: ${OSTYPE}" >&2
        exit 1
        ;;
esac

# Detect architecture
case "$(uname -m)" in
    x86_64|amd64)
        ARCH=amd64
        ;;
    aarch64|arm64)
        ARCH=arm64
        ;;
    *)
        echo "Unsupported architecture: $(uname -m)" >&2
        exit 1
        ;;
esac

# Resolve latest version
if [ "${VERSION}" = "latest" ]; then
    VERSION=$(
        curl -fsSL "${GITHUB_RELEASES_API_URL}/latest" |
            grep '"tag_name":' |
            head -n1 |
            cut -d'"' -f4
    )

    if [ -z "${VERSION}" ]; then
        echo "Failed to determine latest version" >&2
        exit 1
    fi
fi

DOWNLOAD_URL="${GITHUB_RELEASES_URL}/download/${VERSION}/kraken_${OS}_${ARCH}.tar.gz"

echo "Installing kraken ${VERSION} (${OS}/${ARCH})..."

mkdir -p "${INSTALL_DIR}"

curl -fsSL "${DOWNLOAD_URL}" |
    tar -xz -C "${INSTALL_DIR}" kraken

chmod 0755 "${INSTALL_DIR}/kraken"

if [ "${BINARY_NAME}" != "kraken" ]; then
    mv "${INSTALL_DIR}/kraken" "${INSTALL_DIR}/${BINARY_NAME}"
fi

echo "Installed ${INSTALL_DIR}/${BINARY_NAME}"
