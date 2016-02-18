#!/bin/bash
TMP_WORK_DIR=`mktemp -d`

# cleanup_exit removes all resources created during the process and exits with
# the supplied returned code.
cleanup_exit() {
    rm -rf $TMP_WORK_DIR
    exit $1
}

git clone https://github.com/centrifugal/web.git $TMP_WORK_DIR
rm -rf $TMP_WORK_DIR/app/src

if [ -d extras/web ]; then
	rm -rf extras/web
fi

cp -r $TMP_WORK_DIR/app/ extras/web

cleanup_exit 0
