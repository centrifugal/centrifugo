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

# uncomment and modify path if updating from local disk system.
# cp -R /Users/fz/projects/centrifugal/web/ $TMP_WORK_DIR
git clone https://github.com/centrifugal/web.git $TMP_WORK_DIR
rm -rf $TMP_WORK_DIR/app/src

statik -src=$TMP_WORK_DIR/app -dest ./internal/ -package=webui

cleanup_exit 0
