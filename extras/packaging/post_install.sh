#!/bin/bash

BIN_DIR=/usr/bin
DATA_DIR=/var/lib/centrifugo
LOG_DIR=/var/log/centrifugo
SCRIPT_DIR=/usr/lib/centrifugo/scripts

function install_init {
    cp -f $SCRIPT_DIR/initd.sh /etc/init.d/centrifugo
    chmod +x /etc/init.d/centrifugo
}

function install_initel6 {
    cp -f $SCRIPT_DIR/initd.el6.sh /etc/init.d/centrifugo
    chmod +x /etc/init.d/centrifugo    
}

function install_systemd {
    cp -f $SCRIPT_DIR/centrifugo.service /lib/systemd/system/centrifugo.service
    systemctl enable centrifugo
}

function install_upstart {
    cp -f $SCRIPT_DIR/centrifugo.upstart /etc/init/centrifugo.conf
    initctl reload-configuration
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

if [ ! -d $DATA_DIR ]; then
    mkdir -p $DATA_DIR
fi    

if [ ! -d $LOG_DIR ]; then
    mkdir -p $LOG_DIR
fi

chown -R -L centrifugo:centrifugo $DATA_DIR
chown -R -L centrifugo:centrifugo $LOG_DIR

# Add defaults file, if it doesn't exist
if [[ ! -f /etc/default/centrifugo ]]; then
    touch /etc/default/centrifugo
fi

# Generate config with unique secret key, if it doesn't exist
if [[ ! -f /etc/centrifugo/config.json ]]; then
    /usr/bin/centrifugo genconfig -c /etc/centrifugo/config.json
fi    

# Distribution-specific logic
if [[ -f /etc/redhat-release ]]; then
    # RHEL-variant logic
    which systemctl &>/dev/null
    if [[ $? -eq 0 ]]; then
	install_systemd
    else
	# Assuming sysv
	install_initel6
	install_chkconfig
    fi
elif [[ -f /etc/debian_version ]]; then
    # Debian/Ubuntu logic
    which systemctl &>/dev/null
    if [[ $? -eq 0 ]]; then
	install_systemd
    else
        /sbin/init --version >/dev/null 2>/dev/null
        if [[ $? -eq 0 && `/sbin/init --version` =~ upstart ]]; then
	    # Assuming upstart
        install_upstart
        else
        # Assuming sysv
    	install_init
    	install_update_rcd
        fi
    fi
elif [[ -f /etc/os-release ]]; then
    source /etc/os-release
    if [[ $ID = "amzn" ]]; then
	# Amazon Linux logic
	install_init
	install_chkconfig
    fi
fi
