#! /usr/bin/env bash

### BEGIN INIT INFO
# Provides:          centrifugo
# Required-Start:    $all
# Required-Stop:     $remote_fs $syslog
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: Start centrifugo at boot time
### END INIT INFO

# Command-line options that can be set in /etc/default/centrifugo.  These will override
# any config file values. Example: CENTRIFUGO_OPTS="--engine=redis"
DEFAULT=/etc/default/centrifugo

# Daemon options
CENTRIFUGO_OPTS=

# Process name
readonly NAME=centrifugo

# Absolute path to the printf executable.
readonly PRINT=/usr/bin/printf

# Absolute path to the which executable.
readonly WHICH=/usr/bin/which

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

readonly EC_INVALID_ARGUMENT=2
readonly EC_SUPER_USER_ONLY=4
readonly EC_DAEMON_NOT_FOUND=5
readonly EC_RELOADING_FAILED=95
readonly EC_RESTART_STOP_FAILED=96
readonly EC_RESTART_START_FAILED=97
readonly EC_START_FAILED=98
readonly EC_STOP_FAILED=99

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

# Exit the script with an error code.
die() {
    log_end_msg $1
    exit $1
}

# End the script.
end() {
    log_end_msg 0
    exit 0
}

# Display failure message.
fail() {
    log_failure_msg "${NAME}" "$1"
}

# Display success message.
ok() {
    log_success_msg "${NAME}" "$1"
}

# Reloads the service.
reload_service()
{
    start-stop-daemon --stop --signal HUP --pidfile $PIDFILE
    if [ $? -ne 0 ]; then
        fail "failed to reload"
    else
        ok "reloaded"
    fi    
}

# Starts the service.
start_service()
{
    # Check if config file exist
    if [ ! -r $CONFIG ]; then
        fail "config file doesn't exist (or you don't have permission to view)"
        return 1
    fi
    # Bump the file limits, before launching the daemon. These will carry over to
    # launched processes.
    ulimit -n $OPEN_FILE_LIMIT
    if [ $? -ne 0 ]; then
        fail "set open file limit to $OPEN_FILE_LIMIT"
        return 1
    fi    
    start-stop-daemon --start --chuid $GROUP:$USER --quiet --background --make-pidfile --pidfile $PIDFILE --startas /bin/bash -- -c "exec $DAEMON -c $CONFIG $CENTRIFUGO_OPTS >> $STDOUT 2>&1"
    if [ $? -ne 0 ]; then
        fail "failed to start"
    else
        ok "started"
    fi
}

# Stops the service.
stop_service() {
    start-stop-daemon --stop --signal QUIT --retry=TERM/30/KILL/5 --pidfile $PIDFILE
    if [ $? -ne 0 ]; then
        fail "failed to stop"
    else
        ok "stopped"
    fi    
}

# Load the LSB log_* functions.
INCLUDE=/lib/lsb/init-functions
if [ -r "${INCLUDE}" ]
then
    . "${INCLUDE}"
else
    "${PRINT}" '%s: unable to load LSB functions, cannot start service.\n' "${NAME}" 1>&2
    exit ${EC_DAEMON_NOT_FOUND}
fi

# Make sure only one argument was passed to the script.
if [ $# -ne 1 ]
then
    if [ $# -lt 1 -o "$1" = '' ]
        then fail 'action not specified.'
        else fail 'too many arguments.'
    fi
    usage 1>&2
    die ${EC_INVALID_ARGUMENT}
fi
readonly ACTION="$1"

# Check if daemon is a recognized program and get absolute path.
readonly DAEMON=$("${WHICH}" "${NAME}")
if [ ! -x "${DAEMON}" ]
then
    if [ "${ACTION}" = 'stop' ]
    then
        fail 'executable not found: stop request ignored'
        end
    else
        fail "executable not found: cannot ${ACTION} service"
        die ${EC_DAEMON_NOT_FOUND}
    fi
fi

# Check start-stop-daemon
readonly SSD=$("${WHICH}" "start-stop-daemon")
if [ ! -x "${SSD}" ]
then
    fail "start-stop-daemon required"
    end
fi

# Determine the service's status.
start-stop-daemon --status --pidfile "${PIDFILE}" 2>/dev/null 1>/dev/null
readonly STATUS=$?

configtest() {
    $DAEMON checkconfig -c $CONFIG
}

status_q() {
    status_of_proc "${DAEMON}" "${NAME}" >/dev/null 2>&1
}

case $1 in
    start)
        if [ ${STATUS} -eq 0 ]
        then
            ok 'already started.'
        else
            start_service || die ${EC_START_FAILED}
        fi
        ;;
    stop)
        if [ ${STATUS} -eq 0 ]
        then
            stop_service || die ${EC_STOP_FAILED}
        else
            ok 'already stopped.'
        fi
        ;;
    restart)
        if [ ${STATUS} -eq 0 ]
        then
            stop_service || die ${EC_RESTART_STOP_FAILED}
            sleep 1
        fi
        start_service || die ${EC_RESTART_START_FAILED}
        ;;
    configtest)
        $1
        ;;
    status)
        status_of_proc "${DAEMON}" "${NAME}" || exit $?
        ;;
    reload)
        if [ ${STATUS} -eq 0 ]
        then
            configtest
            if [[ $? -ne 0 ]]; then
                die ${EC_RELOADING_FAILED}
            fi
            reload_service || die ${EC_RELOADING_FAILED}
        else
            fail 'service not running, nothing to be done.'
        fi
        ;;        
    version)
        $DAEMON version
        ;;
    condrestart)
        status_q && $0 restart || exit 0
        ;;
    *)
        # For invalid arguments, print the usage message.
        echo "Usage: $0 {start|stop|restart|condrestart|reload|configtest|status|version}"
        exit 2
        ;;
esac
