#!/bin/bash
#
# /etc/init.d/centrifugo
#
# Startup script for centrifugo
#
# chkconfig: 2345 20 80
# description: Starts and stops centrifugo

# Source function library.
if [ -f /etc/init.d/functions ] ; then
    . /etc/init.d/functions
elif [ -f /etc/rc.d/init.d/functions ] ; then
    . /etc/rc.d/init.d/functions
else
    exit 1
fi

# Daemon options
CENTRIFUGO_OPTS=

# Process name
NAME=centrifugo

USER=centrifugo
GROUP=centrifugo

# Daemon name, where is the actual executable
# If the daemon is not there, then exit.
DAEMON=/usr/bin/centrifugo

# Configuration file
CONFIG=/etc/centrifugo/config.json

# Max open files
OPEN_FILE_LIMIT=65536

# Logging
if [ -z "$STDOUT" ]; then
    STDOUT=/var/log/centrifugo/centrifugo.log
fi

if [ ! -f "$STDOUT" ]; then
    mkdir -p $(dirname $STDOUT)
fi

if [ -z "$STDERR" ]; then
    STDERR=/var/log/centrifugo/centrifugo.log
fi

if [ ! -f "$STDERR" ]; then
    mkdir -p $(dirname $STDERR)
fi

lockfile="/var/lock/subsys/centrifugo"
pidfile="/var/run/centrifugo.pid"

[ -e /etc/sysconfig/centrifugo ] && . /etc/sysconfig/centrifugo

if ! [ -x $DAEMON ]; then
  echo "$DAEMON binary not found."
  exit 5
fi

start() {
    if ! [ -f $CONFIG ]; then
        echo "configuration file not found $CONFIG"
        exit 6
    fi

    # Bump the file limits, before launching the daemon.
    ulimit -n $OPEN_FILE_LIMIT
    if [ $? -ne 0 ]; then
        exit 1
    fi

    configtest || return $?
    echo -n $"Starting $NAME: "
    daemon --pidfile=$pidfile --user centrifugo --check=$DAEMON "nohup $DAEMON -c $CONFIG $CENTRIFUGO_OPTS >>$STDOUT 2>&1 &"
    retval=$?
    echo
    [ $retval -eq 0 ] && touch $lockfile
    pgrep -f "$DAEMON -c $CONFIG" > $pidfile
    return $retval
}

stop() {
    echo -n $"Stopping $NAME: "
    killproc -p $pidfile $NAME -TERM
    retval=$?
    if [ $retval -eq 0 ]; then
        if [ "$CONSOLETYPE" != "serial" ]; then
           echo -en "\\033[16G"
        fi
        while rh_status_q
        do
            sleep 1
            echo -n $"."
        done
        rm -f $lockfile
    fi
    echo
    return $retval
}

restart() {
    configtest || return $?
    stop
    sleep 1
    start
}

reload() {
    configtest || return $?
    echo -n $"Reloading $NAME: "
    killproc $DAEMON -HUP
    sleep 1
    RETVAL=$?
    echo
}

configtest() {
    $DAEMON checkconfig -c $CONFIG
}

rh_status() {
    status -p $pidfile $NAME
}

rh_status_q() {
    rh_status >/dev/null 2>&1
}

case "$1" in
    start)
        rh_status_q && exit 0
        $1
        ;;
    stop)
        rh_status_q || exit 0
        $1
        ;;
    restart|configtest)
        $1
        ;;
    reload)
        rh_status_q || exit 7
        $1
        ;;
    status|status_q)
        rh_$1
        ;;
    condrestart)
        rh_status_q && restart || exit 0
        ;;
    *)
        echo $"Usage: $0 {start|stop|reload|status|restart|configtest|condrestart}"
        exit 2
esac
