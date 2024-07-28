#!/bin/bash

# this script updates embedded swagger web interface.
# Script intended to be run from the repo root folder:
# ./misc/scripts/update_swagger_web.sh

TMP_WORK_DIR=$(mktemp -d)

# cleanup_exit removes all resources created during the process and exits with
# the supplied returned code.
cleanup_exit() {
    rm -rf "$TMP_WORK_DIR"
    exit "$1"
}

git clone -c advice.detachedHead=false --depth 1 --branch v4.18.1 --single-branch https://github.com/swagger-api/swagger-ui.git "$TMP_WORK_DIR"
cp internal/apiproto/swagger/api.swagger.json "$TMP_WORK_DIR"/dist/swagger.json

oldString="https://petstore.swagger.io/v2/"
newString="./"
sed -i '' "s|${oldString}|${newString}|g" "$TMP_WORK_DIR"/dist/swagger-initializer.js

statik -src="$TMP_WORK_DIR"/dist -dest ./internal/ -package=swaggerui

cleanup_exit 0
