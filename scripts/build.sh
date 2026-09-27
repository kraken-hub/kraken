#!/bin/bash

SCRIPT_DIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )
WORKDIR="$( dirname "${SCRIPT_DIR}" )"

cd "${WORKDIR}"

GOOS=${GOOS:=linux}
GOARCH=${GOARCH:=amd64}
BUILD_DIR=${BUILD_DIR:=.build}
VERSION=${VERSION:=0.x}
ARCHIVE=NO

ARCH="${GOARCH}"

for i in "$@"; do
case $i in
    --archive)
        ARCHIVE=YES
        shift
    ;;
    *)
        # unknown option
    ;;
esac
done

ARCH_DIR="${GOOS}/${ARCH}"
DIR="${BUILD_DIR}/${ARCH_DIR}"
mkdir -p "${DIR}"

FILE="kraken"

SHA=$( git -c safe.directory="${WORKDIR}" rev-parse HEAD 2>/dev/null | head -c7 )
if [ -z "${SHA}" ]; then
    SHA="dev"
fi

PACKAGE="./cmd/kraken"

CGO_ENABLED=0 go build -buildvcs=false -o "${DIR}/${FILE}" -ldflags "-X main.version=${VERSION}-${SHA}" ${PACKAGE}

md5_sum() {
    local IN_FILE=${1}
    local OUT_FILE=${2}

    if [[ "$OSTYPE" == "darwin"* ]]; then
        md5 "${IN_FILE}" > "${OUT_FILE}"
    else
        md5sum --tag "${IN_FILE}" > "${OUT_FILE}"
    fi
}

cd "${BUILD_DIR}"
md5_sum "${ARCH_DIR}/${FILE}" "${ARCH_DIR}/md5"
cd "${WORKDIR}"

if [ "YES" = "${ARCHIVE}" ]; then
    cd "${DIR}"
    cp "${WORKDIR}/LICENSE" ./
    cp "${WORKDIR}/README.md" ./
    TAR_FILE="kraken_${GOOS}_${ARCH}.tar.gz"
    tar -czf "${WORKDIR}/${BUILD_DIR}/${TAR_FILE}" *
    cd "${WORKDIR}/${BUILD_DIR}"
    md5_sum "${TAR_FILE}" "${TAR_FILE}.md5"
    rm -rf "${WORKDIR}/$( dirname "${DIR}" )"
fi
