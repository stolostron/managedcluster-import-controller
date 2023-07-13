#!/bin/bash
# Copyright Contributors to the Open Cluster Management project

# These functions help to trap a signal, kill the child process.
# These functions are intended to run in a POD.

# Trap a signal and call the provided function
# Parameter 1: The signal number (ie: 15)
# Parameter 2: The function to call
# Extra parameters are passed as parameters of the function.
trap_with_arg() {
    func="$1" ; shift
    sig=$1
    trap "$func $*" "$sig"
}

# This function traps a signal
# Parameter 1: The signal to use while killed the child process
# Parameter 2: The PID of the child process
# Parameter 3: The coverage file path
func_trap() {
    trap=$1
    pid="$2"
    filePath="$3"

    #Generate the coverage data
    echo "Save coverage data to $filePath"
    kill -$trap $pid
    wait_data $filePath
}

# This function wait if the coverage data are posted in the POD.
wait_data() {
   n="10"
    while [ $n != 0 ]; do
        if [ -f $1 ]; then
            break
        fi
        echo "Coverage data not posted yet..."
        sleep 5
        n=$[$n-1]
    done
}
