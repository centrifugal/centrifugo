#!/bin/bash

# this script updates embedded web interface. 
# Script intended to be run from the repo root folder:
# ./misc/scripts/update_web.sh

TMP_WORK_DIR=`mktemp -d`

# cleanup_exit removes all resources created during the process and exits with
# the supplied returned code.
cleanup_exit() {
    rm -rf $TMP_WORK_DIR
    exit $1
}

git clone --depth 1 https://github.com/centrifugal/web.git $TMP_WORK_DIR
rsync -av --delete "$TMP_WORK_DIR"/dist ./internal/webui/web

cleanup_exit 0
