#!/bin/sh
# 
# Debian 7: lsb sysv
# Debian 8: systemd
# Ubuntu 14.04: upstart
# Ubuntu 16.04: systemd
# Centos 6: sysv
# Centos 7: systemd
#
if [ "$1" = "" ]
then
  echo "Usage: $0 <version> <iteration>"
  exit
fi

if [ "$2" = "" ]
then
  echo "Usage: $0 <version> <iteration>"
  exit
fi

VERSION=$1
ITERATION=$2
UNIX_NOW=$( date +%s )

INSTALL_DIR=/usr/bin
LOG_DIR=/var/log/centrifugo
CONFIG_DIR=/etc/centrifugo
LOGROTATE_DIR=/etc/logrotate.d
DATA_DIR=/var/lib/centrifugo
SCRIPT_DIR=/usr/lib/centrifugo

SAMPLE_CONFIGURATION=misc/packaging/config.json
INITD_SCRIPT=misc/packaging/initd.sh
INITD_EL6_SCRIPT=misc/packaging/initd.el6.sh
UPSTART_SCRIPT=misc/packaging/centrifugo.upstart
SYSTEMD_SCRIPT=misc/packaging/centrifugo.service
POSTINSTALL_SCRIPT=misc/packaging/post_install.sh
PREINSTALL_SCRIPT=misc/packaging/pre_install.sh
POSTUNINSTALL_SCRIPT=misc/packaging/post_uninstall.sh
LOGROTATE=misc/packaging/logrotate

TMP_WORK_DIR=`mktemp -d`
TMP_BINARIES_DIR=`mktemp -d`
ARCH=amd64
NAME=centrifugo
LICENSE=MIT
URL="https://github.com/centrifugal/centrifugo"
MAINTAINER="frvzmb@gmail.com"
VENDOR=centrifugo
DESCRIPTION="Real-time messaging server"

echo "Start packaging, version: $VERSION, iteration: $ITERATION, release time: $UNIX_NOW"

# cleanup_exit removes all resources created during the process and exits with
# the supplied returned code.
cleanup_exit() {
    rm -r $TMP_WORK_DIR
    rm -r $TMP_BINARIES_DIR
    exit $1
}

# make_dir_tree creates the directory structure within the packages.
make_dir_tree() {
    work_dir=$1

    mkdir -p $work_dir/$INSTALL_DIR
    if [ $? -ne 0 ]; then
        echo "Failed to create install directory -- aborting."
        cleanup_exit 1
    fi
    mkdir -p $work_dir/$SCRIPT_DIR/scripts
    if [ $? -ne 0 ]; then
        echo "Failed to create script directory -- aborting."
        cleanup_exit 1
    fi
    mkdir -p $work_dir/$CONFIG_DIR
    if [ $? -ne 0 ]; then
        echo "Failed to create configuration directory -- aborting."
        cleanup_exit 1
    fi
    mkdir -p $work_dir/$LOGROTATE_DIR
    if [ $? -ne 0 ]; then
        echo "Failed to create logrotate directory -- aborting."
        cleanup_exit 1
    fi
}

# do_build builds the code. The version and commit must be passed in.
do_build() {
    echo "Start building binary"

    if [ -z "$STATS_ENDPOINT" ]
    then
        echo "STATS_ENDPOINT not defined"
        cleanup_exit 1
    else
        echo "STATS_ENDPOINT found"
    fi

    if [ -z "$STATS_TOKEN" ]
    then
        echo "STATS_TOKEN not defined"
        cleanup_exit 1
    else
        echo "STATS_TOKEN found"
    fi

    CGO_ENABLED=0 GOOS="linux" GOARCH="amd64" go build -ldflags="-X github.com/centrifugal/centrifugo/v6/internal/build.Version=$VERSION -X github.com/centrifugal/centrifugo/v6/internal/build.UsageStatsEndpoint=$STATS_ENDPOINT -X github.com/centrifugal/centrifugo/v6/internal/build.UsageStatsToken=$STATS_TOKEN" -o "$TMP_BINARIES_DIR/centrifugo"
    echo "Binary build completed successfully"
    "$TMP_BINARIES_DIR"/centrifugo version
}

do_build $VERSION

make_dir_tree $TMP_WORK_DIR

cp $TMP_BINARIES_DIR/centrifugo $TMP_WORK_DIR/$INSTALL_DIR/
if [ $? -ne 0 ]; then
    echo "Failed to copy binaries to packaging directory ($TMP_WORK_DIR/$INSTALL_DIR/) -- aborting."
    cleanup_exit 1
fi

echo "centrifugo binary copied to $TMP_WORK_DIR/$INSTALL_DIR/"

cp $INITD_SCRIPT $TMP_WORK_DIR/$SCRIPT_DIR/scripts/initd.sh
if [ $? -ne 0 ]; then
    echo "Failed to copy init.d script to packaging directory ($TMP_WORK_DIR/$SCRIPT_DIR/) -- aborting."
    cleanup_exit 1
fi

echo "initd script copied to $TMP_WORK_DIR/$SCRIPT_DIR/scripts"

cp $INITD_EL6_SCRIPT $TMP_WORK_DIR/$SCRIPT_DIR/scripts/initd.el6.sh
if [ $? -ne 0 ]; then
    echo "Failed to copy initd.el6.sh script to packaging directory ($TMP_WORK_DIR/$SCRIPT_DIR/) -- aborting."
    cleanup_exit 1
fi

echo "initd.el6 script copied to $TMP_WORK_DIR/$SCRIPT_DIR/scripts"

cp $SYSTEMD_SCRIPT $TMP_WORK_DIR/$SCRIPT_DIR/scripts/centrifugo.service
if [ $? -ne 0 ]; then
    echo "Failed to copy systemd script to packaging directory -- aborting."
    cleanup_exit 1
fi

echo "systemd script copied to $TMP_WORK_DIR/$SCRIPT_DIR/scripts"

cp $UPSTART_SCRIPT $TMP_WORK_DIR/$SCRIPT_DIR/scripts/centrifugo.upstart
if [ $? -ne 0 ]; then
    echo "Failed to copy upstart script to packaging directory -- aborting."
    cleanup_exit 1
fi

echo "upstart script copied to $TMP_WORK_DIR/$SCRIPT_DIR/scripts"

install -m 644 $LOGROTATE $TMP_WORK_DIR/$LOGROTATE_DIR/centrifugo
if [ $? -ne 0 ]; then
    echo "Failed to copy logrotate configuration to packaging directory -- aborting."
    cleanup_exit 1
fi

echo "logrotate script copied to $TMP_WORK_DIR/$LOGROTATE_DIR/centrifugo"

COMMON_FPM_ARGS="\
-C $TMP_WORK_DIR \
--log error \
--version $VERSION \
--iteration $ITERATION \
--name $NAME \
--vendor $VENDOR \
--url $URL \
--category Network \
--license $LICENSE \
--maintainer $MAINTAINER \
--force \
--after-install $POSTINSTALL_SCRIPT \
--before-install $PREINSTALL_SCRIPT \
--after-remove $POSTUNINSTALL_SCRIPT \
--config-files $CONFIG_DIR \
--config-files $LOGROTATE_DIR "

echo "Start building rpm package"

rm -r ./PACKAGES
mkdir -p PACKAGES

fpm -s dir -t rpm $COMMON_FPM_ARGS --description "$DESCRIPTION" \
    --rpm-os linux \
    -p PACKAGES/ \
    -a amd64 .

echo "Start building deb package"

fpm -s dir -t deb $COMMON_FPM_ARGS --description "$DESCRIPTION" \
    -p PACKAGES/ \
    -a amd64 .

cd PACKAGES
for f in *.deb; do sha256sum $f >> ${f}_checksum.txt; done
for f in *.rpm; do sha256sum $f >> ${f}_checksum.txt; done
cd ..

echo "Packaging complete!"

cleanup_exit 0
