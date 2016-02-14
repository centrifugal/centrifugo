#! /usr/bin/env bash

### BEGIN INIT INFO
# Provides:          centrifugo
# Required-Start:    $all
# Required-Stop:     $remote_fs $syslog
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: Start centrifugo at boot time
### END INIT INFO

# If you modify this, please make sure to also edit centrifugo.service
# this init script supports three different variations:
#  1. New lsb that define start-stop-daemon
#  2. Old lsb that don't have start-stop-daemon but define, log, pidofproc and killproc
#  3. Centos installations without lsb-core installed
#
# In the third case we have to define our own functions which are very dumb
# and expect the args to be positioned correctly.

# Command-line options that can be set in /etc/default/centrifugo.  These will override
# any config file values. Example: CENTRIFUGO_OPTS="--engine=redis"
DEFAULT=/etc/default/centrifugo

# Daemon options
CENTRIFUGO_OPTS=

# Process name ( For display )
NAME=centrifugo

# User and group
USER=centrifugo
GROUP=centrifugo

# Daemon name, where is the actual executable
# If the daemon is not there, then exit.
DAEMON=/usr/bin/centrifugo
[ -x $DAEMON ] || exit 5

# Configuration file
CONFIG=/etc/centrifugo/config.json

# PID file for the daemon
PIDFILE=/var/run/centrifugo/centrifugo.pid
PIDDIR=`dirname $PIDFILE`
if [ ! -d "$PIDDIR" ]; then
    mkdir -p $PIDDIR
    chown $USER:$GROUP $PIDDIR
fi

# Max open files
OPEN_FILE_LIMIT=65536

if [ -r /lib/lsb/init-functions ]; then
    source /lib/lsb/init-functions
fi

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

# Overwrite init script variables with /etc/default/centrifugo values
if [ -r $DEFAULT ]; then
    source $DEFAULT
fi

function pidofproc() {
    if [ $# -ne 3 ]; then
        echo "Expected three arguments, e.g. $0 -p pidfile daemon-name"
    fi

    PID=`pgrep -f $3`
    local PIDFILE=`cat $2`

    if [ "x$PIDFILE" == "x" ]; then
        return 1
    fi

    if [ "x$PID" != "x" -a "$PIDFILE" == "$PID" ]; then
        return 0
    fi

    return 1
}

function killproc() {
    if [ $# -ne 3 ]; then
        echo "Expected three arguments, e.g. $0 -p pidfile signal"
    fi

    PID=`cat $2`

    /bin/kill -s $3 $PID
    while true; do
        pidof `basename $DAEMON` >/dev/null
        if [ $? -ne 0 ]; then
            return 0
        fi

        sleep 1
        n=$(expr $n + 1)
        if [ $n -eq 30 ]; then
            /bin/kill -s SIGKILL $PID
            return 0
        fi
    done
}

function log_failure_msg() {
    echo "$@" "[ FAILED ]"
}

function log_success_msg() {
    echo "$@" "[ OK ]"
}

start() {
    # Check if config file exist
    if [ ! -r $CONFIG ]; then
        log_failure_msg "config file doesn't exist (or you don't have permission to view)"
        exit 4
    fi

    # Checked the PID file exists and check the actual status of process
    if [ -e $PIDFILE ]; then
    PID="$(pgrep -f $PIDFILE)"
    if test ! -z $PID && kill -0 "$PID" &>/dev/null; then
    # If the status is SUCCESS then don't need to start again.
            log_failure_msg "$NAME is running"
            exit 0 # Exit
        fi
    # if PID file does not exist, check if writable
    else
        su -s /bin/sh -c "touch $PIDFILE" $USER > /dev/null 2>&1
        if [ $? -ne 0 ]; then
            log_failure_msg "$PIDFILE not writable, check permissions"
            exit 5
        fi
    fi

    # Bump the file limits, before launching the daemon. These will carry over to
    # launched processes.
    ulimit -n $OPEN_FILE_LIMIT
    if [ $? -ne 0 ]; then
        log_failure_msg "set open file limit to $OPEN_FILE_LIMIT"
        exit 1
    fi

    if which start-stop-daemon > /dev/null 2>&1; then
        start-stop-daemon --chuid $GROUP:$USER --start --quiet --pidfile $PIDFILE --exec $DAEMON -- -c $CONFIG $CENTRIFUGO_OPTS >>$STDOUT 2>>&1 &
    else
        su -s /bin/sh -c "nohup $DAEMON -c $CONFIG $CENTRIFUGO_OPTS >>$STDOUT 2>>&1 &" $USER
    fi
    retval=$?
    echo
    return $retval
}

stop() {
    echo -n $"Stopping $NAME"
    if [ -e $PIDFILE ]; then
    PID="$(pgrep -f $PIDFILE)"
    if test ! -z $PID && kill -0 "$PID" &>/dev/null; then
            if killproc -p $PIDFILE SIGTERM && /bin/rm -rf $PIDFILE; then
                log_success_msg "$NAME was stopped"
                return 0
            else
                log_failure_msg "$NAME failed to stop"
                return 1
            fi
        fi
    else
        log_failure_msg "$NAME is not running"
        return 1
    fi    
}

restart() {
    configtest || return $?
    $0 stop && sleep 1 && $0 start
}

reload() {
    configtest || return $?
    echo -n $"Reloading $NAME: "
    kill -HUP $(cat /var/run/centrifugo/centrifugo.pid)
    sleep 1
    RETVAL=$?
    echo
}

configtest() {
    $DAEMON checkconfig -c $CONFIG
}

status() {
    if [ -e $PIDFILE ]; then
        PID="$(pgrep -f $PIDFILE)"
        if test ! -z $PID && test -d "/proc/$PID" &>/dev/null; then
            log_success_msg "$NAME is running"
            exit 0
        else
            log_failure_msg "$NAME is not running"
            exit 1
        fi
    else
        log_failure_msg "$NAME is not running"
        exit 3
    fi
}

status_q() {
    status >/dev/null 2>&1
}

case $1 in
    start)
        status_q && exit 0
        $1
        ;;
    stop)
        status_q || exit 0
        $1
        ;;
    restart|configtest)
        $1
        ;;
    status)
        $1
        ;;
    reload)
        $0 status || exit 7
        $1
        ;;        
    version)
        $DAEMON version
        ;;
    condrestart)
        $0 status && $0 restart || exit 0
        ;;
    *)
        # For invalid arguments, print the usage message.
        echo "Usage: $0 {start|stop|restart|condrestart|reload|configtest|status|version}"
        exit 2
        ;;
esac
