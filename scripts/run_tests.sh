#!/usr/bin/env bash

SCRIPTS_DIR="$( dirname "${BASH_SOURCE[0]}" )"
RELEASE_DIR="${SCRIPTS_DIR}/.."

ginkgo -r "${RELEASE_DIR}/src/migrate-to-pxc/disk/"