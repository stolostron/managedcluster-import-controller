#!/bin/bash

# Total "minutes" to retry before giving up
CLUSTERPOOL_CHECKOUT_TIMEOUT_MINUTES=$1

if [ "$CLUSTERPOOL_CHECKOUT_TIMEOUT_MINUTES" = "" ]; then CLUSTERPOOL_CHECKOUT_TIMEOUT_MINUTES=10
fi

# We retry on 30-second intervals (plus query overhead...)
RETRIES=$(( 2*$CLUSTERPOOL_CHECKOUT_TIMEOUT_MINUTES ))

make clusterpool/_create-claim
make clusterpool/_gather-status

if [ "`cat .verifyStatus`" = "ClusterReady" ]; then cat .verifyStatus; else
	if [ ! "`cat .verifyStatus`" = "ClusterReady" ]; then
		echo Waiting $CLUSTERPOOL_CHECKOUT_TIMEOUT_MINUTES minutes for cluster availability...
		cat .verifyStatus
		for (( i=1; i<=$RETRIES; i++ ))
		do
			sleep 30
			make clusterpool/_gather-status
			cat .verifyStatus
			if [ "`cat .verifyStatus`" = "ClusterReady" ]; then exit 0; fi
		done
	else
		echo Unknown claim status `cat .verifyStatus` - exiting
		# Unknown claim state - delete it
		make clusterpool/_delete-claim
		exit 1
	fi
	echo Claim provision timed out - exiting
	# We never got our claim satisfied - delete it
	make clusterpool/_delete-claim
	exit 1
fi
