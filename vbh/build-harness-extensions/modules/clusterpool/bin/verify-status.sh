#!/bin/bash

# input:
#   $1 - ClusterClaim json
#   $2 - ClusterDeployment json

NAMESPACE=`jq -r '.spec.namespace' $1`
CC_PEND_CONDITION=`jq -r '.status.conditions[]? | select(.type=="Pending").status' $1`
CC_PEND_REASON=`jq -r '.status.conditions[]? | select(.type=="Pending").reason' $1`
CD_HIB_CONDITION=`jq -r '.status.conditions[]? | select(.type=="Hibernating") | .status' $2`
CD_HIB_REASON=`jq -r '.status.conditions[]? | select(.type=="Hibernating") | .reason' $2`
CD_UNR_CONDITION=`jq -r '.status.conditions[]? | select(.type=="Unreachable") | .status' $2`
CD_UNR_REASON=`jq -r '.status.conditions[]? | select(.type=="Unreachable") | .reason' $2`
#echo namespace: "$NAMESPACE"
#echo cc_pend_condition: "$CC_PEND_CONDITION"
#echo cd_pend_reason: "$CC_PEND_REASON"
#echo cd_hib_condition: "$CD_HIB_CONDITION"
#echo cd_hib_reason: "$CD_HIB_REASON"
#echo cd_unr_condition: "$CD_UNR_CONDITION"
#echo cd_unr_reason: "$CD_UNR_REASON"
if [[ ! "$NAMESPACE" = "null" && "$CC_PEND_CONDITION" = "False" && "$CD_HIB_CONDITION" = "False" && "$CD_UNR_CONDITION" = "False" ]]; then
        echo ClusterReady > .verifyStatus;
        else
        echo ClusterNotReady - [Pending: $CC_PEND_CONDITION:$CC_PEND_REASON] [Hibernating: $CD_HIB_CONDITION:$CD_HIB_REASON] [Unreachable: $CD_UNR_CONDITION:$CD_UNR_REASON] > .verifyStatus
fi
