#!/bin/sh
if [ "$1" = "" ]
then
  echo "Usage: $0 <version>"
  exit
fi

INSTALL_DIR=/usr/bin
LOG_DIR=/var/log/centrifugo
CONFIG_DIR=/etc/centrifugo
LOGROTATE_DIR=/etc/logrotate.d
DATA_DIR=/var/lib/centrifugo
SCRIPT_DIR=/usr/lib/centrifugo

SAMPLE_CONFIGURATION=extras/packaging/config.json
INITD_SCRIPT=extras/packaging/initd.sh
SYSTEMD_SCRIPT=extras/packaging/centrifugo.service
POSTINSTALL_SCRIPT=extras/packaging/post_install.sh
PREINSTALL_SCRIPT=extras/packaging/pre_install.sh
POSTUNINSTALL_SCRIPT=extras/packaging/post_uninstall.sh
LOGROTATE=extras/packaging/logrotate

TMP_WORK_DIR=`mktemp -d`
POST_INSTALL_PATH=`mktemp`
POST_UNINSTALL_PATH=`mktemp`
ARCH=`amd64`
NAME=centrifugo
LICENSE=MIT
URL="https://github.com/centrifugal/centrifugo"
MAINTAINER="frvzmb@gmail.com"
VENDOR=Centrifugo
DESCRIPTION="Real-time messaging server"
VERSION=$1

echo $TMP_WORK_DIR

# check_gopath checks the GOPATH env variable set
check_gopath() {
    [ -z "$GOPATH" ] && echo "GOPATH is not set." && cleanup_exit 1
    echo "GOPATH: $GOPATH"
}

# cleanup_exit removes all resources created during the process and exits with
# the supplied returned code.
cleanup_exit() {
    rm -r $TMP_WORK_DIR
    rm $POST_INSTALL_PATH
    rm $POST_UNINSTALL_PATH
    exit $1
}

# make_dir_tree creates the directory structure within the packages.
make_dir_tree() {
    work_dir=$1
    version=$2

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
	gox -os="linux" -arch="amd64" -output="$TMP_WORK_DIR/binaries/{{.OS}}-{{.Arch}}/{{.Dir}}"
    echo "Build completed successfully."
}

# Start!
echo "Starting packaging..."

check_gopath

do_build $VERSION

make_dir_tree $TMP_WORK_DIR $VERSION

cp $TMP_WORK_DIR/binaries/linux-amd64/centrifugo $TMP_WORK_DIR/$INSTALL_DIR/
if [ $? -ne 0 ]; then
    echo "Failed to copy binaries to packaging directory ($TMP_WORK_DIR/$INSTALL_DIR/) -- aborting."
    cleanup_exit 1
fi

rm -r $TMP_WORK_DIR/binaries

echo "centrifugo binary copied to $TMP_WORK_DIR/$INSTALL_DIR/"

cp $INITD_SCRIPT $TMP_WORK_DIR/$SCRIPT_DIR/scripts/initd.sh
if [ $? -ne 0 ]; then
    echo "Failed to copy init.d script to packaging directory ($TMP_WORK_DIR/$SCRIPT_DIR/) -- aborting."
    cleanup_exit 1
fi

echo "$INITD_SCRIPT copied to $TMP_WORK_DIR/$SCRIPT_DIR/scripts"

cp $SYSTEMD_SCRIPT $TMP_WORK_DIR/$SCRIPT_DIR/scripts/centrifugo.service
if [ $? -ne 0 ]; then
    echo "Failed to copy systemd script to packaging directory -- aborting."
    cleanup_exit 1
fi

echo "$SYSTEMD_SCRIPT copied to $TMP_WORK_DIR/$SCRIPT_DIR/scripts"

cp $SAMPLE_CONFIGURATION $TMP_WORK_DIR/$CONFIG_DIR/config.json
if [ $? -ne 0 ]; then
    echo "Failed to copy $SAMPLE_CONFIGURATION to packaging directory -- aborting."
    cleanup_exit 1
fi

echo "$SAMPLE_CONFIGURATION copied to $TMP_WORK_DIR/$CONFIG_DIR/config.json"

install -m 644 $LOGROTATE $TMP_WORK_DIR/$LOGROTATE_DIR/centrifugo
if [ $? -ne 0 ]; then
    echo "Failed to copy logrotate configuration to packaging directory -- aborting."
    cleanup_exit 1
fi

echo "$LOGROTATE copied to $TMP_WORK_DIR/$LOGROTATE_DIR/centrifugo"

COMMON_FPM_ARGS="\
-v $VERSION \
--log error \
-C $TMP_WORK_DIR \
--vendor $VENDOR \
--url $URL \
--category Network \
--license $LICENSE \
--maintainer $MAINTAINER \
--after-install $POSTINSTALL_SCRIPT \
--before-install $PREINSTALL_SCRIPT \
--after-remove $POSTUNINSTALL_SCRIPT \
--name $NAME \
--config-files $CONFIG_DIR \
--config-files $LOGROTATE_DIR "

echo $COMMON_FPM_ARGS

fpm -s dir -t rpm $COMMON_FPM_ARGS --description "$DESCRIPTION" --rpm-os linux -a amd64 .

fpm -s dir -t deb $COMMON_FPM_ARGS --description "$DESCRIPTION" -a amd64 .

echo "Packaging process complete!"
cleanup_exit 0
