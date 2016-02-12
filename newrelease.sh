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
ARCH=`uname -a`
NAME=centrifugo
LICENSE=MIT
URL="https://github.com/centrifugal/centrifugo"
MAINTAINER="Alexandr Emelin <frvzmb@gmail.com>"
VENDOR=Centrifugo
DESCRIPTION="Real-time messaging server"
VERSION=$1

GO_VERSION="go1.5.3"
GOPATH_INSTALL=
BINS=(
    centrifugo
    )


# check_gopath sanity checks the value of the GOPATH env variable, and determines
# the path where build artifacts are installed. GOPATH may be a colon-delimited
# list of directories.
check_gopath() {
    [ -z "$GOPATH" ] && echo "GOPATH is not set." && cleanup_exit 1
    GOPATH_INSTALL=`echo $GOPATH | cut -d ':' -f 1`
    [ ! -d "$GOPATH_INSTALL" ] && echo "GOPATH_INSTALL is not a directory." && cleanup_exit 1
    echo "GOPATH ($GOPATH) looks sane, using $GOPATH_INSTALL for installation."
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
    for b in ${BINS[*]}; do
        rm -f $GOPATH_INSTALL/bin/$b
    done

    if [ -n "$WORKING_DIR" ]; then
        STASH=`git stash create -a`
        if [ $? -ne 0 ]; then
            echo "WARNING: failed to stash uncommited local changes"
        fi
        git reset --hard
    fi

    go get -u -f -d ./...
    if [ $? -ne 0 ]; then
        echo "WARNING: failed to 'go get' packages."
    fi

    git checkout $TARGET_BRANCH # go get switches to master, so ensure we're back.

    if [ -n "$WORKING_DIR" ]; then
        git stash apply $STASH
        if [ $? -ne 0 ]; then #and apply previous uncommited local changes
            echo "WARNING: failed to restore uncommited local changes"
        fi
    fi

    version=$1
    commit=`git rev-parse HEAD`
    branch=`current_branch`
    if [ $? -ne 0 ]; then
        echo "Unable to retrieve current commit -- aborting"
        cleanup_exit 1
    fi

    date=`date -u --iso-8601=seconds`
    go install $RACE -a -ldflags="-X main.version=$version -X main.branch=$branch -X main.commit=$commit -X main.buildTime=$date" ./...
    if [ $? -ne 0 ]; then
        echo "Build failed, unable to create package -- aborting"
        cleanup_exit 1
    fi
    echo "Build completed successfully."
}

# Start!
echo -e "\nStarting packaging...\n"

check_gopath

do_build $VERSION

make_dir_tree $TMP_WORK_DIR `$VERSION`

for b in ${BINS[*]}; do
    cp $GOPATH_INSTALL/bin/$b $TMP_WORK_DIR/$INSTALL_ROOT_DIR/
    if [ $? -ne 0 ]; then
        echo "Failed to copy binaries to packaging directory ($TMP_WORK_DIR/$INSTALL_DIR/) -- aborting."
        cleanup_exit 1
    fi
done

echo "${BINS[*]} copied to $TMP_WORK_DIR/$INSTALL_DIR/"

cp $INITD_SCRIPT $TMP_WORK_DIR/$SCRIPT_DIR/$INITD_SCRIPT
if [ $? -ne 0 ]; then
    echo "Failed to copy init.d script to packaging directory ($TMP_WORK_DIR/$SCRIPT_DIR/) -- aborting."
    cleanup_exit 1
fi

echo "$INITD_SCRIPT copied to $TMP_WORK_DIR/$SCRIPT_DIR"

cp $SYSTEMD_SCRIPT $TMP_WORK_DIR/$SCRIPT_DIR/$SYSTEMD_SCRIPT
if [ $? -ne 0 ]; then
    echo "Failed to copy systemd script to packaging directory -- aborting."
    cleanup_exit 1
fi

echo "$SYSTEMD_SCRIPT copied to $TMP_WORK_DIR/$SCRIPT_DIR"

cp $SAMPLE_CONFIGURATION $TMP_WORK_DIR/$CONFIG_DIR/config.json
if [ $? -ne 0 ]; then
    echo "Failed to copy $SAMPLE_CONFIGURATION to packaging directory -- aborting."
    cleanup_exit 1
fi

install -m 644 $LOGROTATE $TMP_WORK_DIR/$LOGROTATE_DIR/centrifugo
if [ $? -ne 0 ]; then
    echo "Failed to copy logrotate configuration to packaging directory -- aborting."
    cleanup_exit 1
fi

COMMON_FPM_ARGS="\
-v $VERSION \
--log error \
-C $TMP_WORK_DIR \
--vendor $VENDOR \
--url $URL \
--category Network \
--license $LICENSE \
--maintainer $MAINTAINER \
--description $DESCRIPTION \
--after-install $POSTINSTALL_SCRIPT \
--before-install $PREINSTALL_SCRIPT \
--after-remove $POSTUNINSTALL_SCRIPT \
--name $NAME \
--config-files $CONFIG_DIR \
--config-files $LOGROTATE_DIR"

fpm -s dir -t rpm $COMMON_FPM_ARGS --rpm-os linux -a amd64 .

fpm -s dir -t deb $COMMON_FPM_ARGS -a amd64 .

echo -e "\nPackaging process complete."
cleanup_exit 0
