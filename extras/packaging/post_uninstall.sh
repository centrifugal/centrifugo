#!/bin/bash

function disable_systemd {
	systemctl stop centrifugo || :
    systemctl disable centrifugo
    rm -f /lib/systemd/system/centrifugo.service
}

function disable_upstart {
	initctl stop centrifugo || :
	rm -f /etc/init/centrifugo.conf
    initctl reload-configuration
}

function disable_update_rcd {
	service centrifugo stop || :
    update-rc.d -f centrifugo remove
    rm -f /etc/init.d/centrifugo
}

function disable_chkconfig {
	service centrifugo stop || :
    chkconfig --del centrifugo
    rm -f /etc/init.d/centrifugo
}

if [[ -f /etc/redhat-release ]]; then
    # RHEL-variant logic
    if [[ "$1" = "0" ]]; then
	# Centrifugo is no longer installed, remove from init system
	which systemctl &>/dev/null
	if [[ $? -eq 0 ]]; then
	    disable_systemd
	else
	    # Assuming sysv
	    disable_chkconfig
	fi
	rm -f /etc/default/centrifugo
    fi
elif [[ -f /etc/debian_version ]]; then
    # Debian/Ubuntu logic
    if [[ "$1" != "upgrade" ]]; then
	# Remove/purge
	which systemctl &>/dev/null
	if [[ $? -eq 0 ]]; then
	    disable_systemd
	else
        /sbin/init --version >/dev/null 2>/dev/null
        if [[ $? -eq 0 && `/sbin/init --version` =~ upstart ]]; then
		# Assuming upstart
		disable_upstart
		else
	    # Assuming sysv
	    disable_update_rcd
		fi
	fi
	rm -f /etc/default/centrifugo
    fi
elif [[ -f /etc/os-release ]]; then
    source /etc/os-release
    if [[ $ID = "amzn" ]]; then
	# Amazon Linux logic
	if [[ "$1" = "0" ]]; then
	    # Centrifugo is no longer installed, remove from init system
	    disable_chkconfig
	    rm -f /etc/default/centrifugo
	fi
    fi
fi
