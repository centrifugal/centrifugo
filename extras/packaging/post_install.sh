#!/bin/bash

BIN_DIR=/usr/bin
DATA_DIR=/var/lib/centrifugo
LOG_DIR=/var/log/centrifugo
SCRIPT_DIR=/usr/lib/centrifugo/scripts
LOGROTATE_DIR=/etc/logrotate.d

function install_init {
    cp -f $SCRIPT_DIR/initd.sh /etc/init.d/centrifugo
    chmod +x /etc/init.d/centrifugo
}

function install_systemd {
    cp -f $SCRIPT_DIR/centrifugo.service /lib/systemd/system/centrifugo.service
    systemctl enable centrifugo
}

function install_update_rcd {
    update-rc.d centrifugo defaults
}

function install_chkconfig {
    chkconfig --add centrifugo
}

id centrifugo &>/dev/null
if [[ $? -ne 0 ]]; then
    useradd --system -U -M centrifugo -s /bin/false -d $DATA_DIR
fi

chown -R -L centrifugo:centrifugo $DATA_DIR
chown -R -L centrifugo:centrifugo $LOG_DIR

# Add defaults file, if it doesn't exist
if [[ ! -f /etc/default/centrifugo ]]; then
    touch /etc/default/centrifugo
fi

# Distribution-specific logic
if [[ -f /etc/redhat-release ]]; then
    # RHEL-variant logic
    which systemctl &>/dev/null
    if [[ $? -eq 0 ]]; then
	install_systemd
    else
	# Assuming sysv
	install_init
	install_chkconfig
    fi
elif [[ -f /etc/debian_version ]]; then
    # Debian/Ubuntu logic
    which systemctl &>/dev/null
    if [[ $? -eq 0 ]]; then
	install_systemd
    else
	# Assuming sysv
	install_init
	install_update_rcd
    fi
elif [[ -f /etc/os-release ]]; then
    source /etc/os-release
    if [[ $ID = "amzn" ]]; then
	# Amazon Linux logic
	install_init
	install_chkconfig
    fi
fi