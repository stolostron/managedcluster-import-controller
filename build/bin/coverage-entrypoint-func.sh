#!/bin/bash
# These functions help to trap a signal, kill the child process and post the code coverage files to an AWS bucket.
# These functions are intended to run in a POD.


# This function etrieves the parameters to access the AWS S3 bucket.
# The parameters are stored in a secret aws-s3-coverage in the open-cluster-management namespace.
# It uses the POD internal hostname to connect with kube and so must run in a POD.
function getAWSParams() {
    APISERVER=https://kubernetes.default.svc
    SERVICEACCOUNT=/var/run/secrets/kubernetes.io/serviceaccount
    TOKEN=$(cat ${SERVICEACCOUNT}/token)
    CACERT=${SERVICEACCOUNT}/ca.crt
    SECRET=$(curl --cacert ${CACERT} --header "Authorization: Bearer ${TOKEN}" -X GET ${APISERVER}/api/v1/namespaces/open-cluster-management/secrets/aws-s3-coverage)

    AWS_ACCESS_KEY_ID=$(echo ${SECRET} | jq -r .data.aws_access_key_id | base64 -d)
    AWS_SECRET_ACCESS_KEY=$(echo ${SECRET} | jq -r .data.aws_secret_access_key | base64 -d)

    AWS_BUCKET_NAME_S=$(echo ${SECRET} | jq -r .data.aws_bucket_name)
    if [ -n "$AWS_BUCKET_NAME_S" ]; then
    AWS_BUCKET_NAME=$(echo $AWS_BUCKET_NAME_S | base64 -d)
    fi

    AWS_BUCKET_FOLDER_S=$(echo ${SECRET} | jq -r .data.aws_bucket_folder)
    if [ -n "$AWS_BUCKET_FOLDER_S" ]; then
    AWS_BUCKET_FOLDER=$(echo $AWS_BUCKET_FOLDER_S | base64 -d)
    fi

    AWS_REGION_S=$(echo ${SECRET} | jq -r .data.aws_AWS_REGION)
    if [ -n "$AWS_REGION_S" ]; then
    AWS_REGION="us-east-2"
    fi
}

# Caclustate the HMAC SHA256 for AWS requests.
function hmac_sha256 {
  key="$1"
  data="$2"
  printf         '%s' "$data" | openssl dgst -sha256 -hex -mac HMAC -macopt "${key}"     2>/dev/null | sed 's/^.* //'
}

# Upload a file to AWS S3
function put_aws {
    fileLocal=$1
    fileRemote="$2"
    awsAccessKeyID="$3"
    awsSecretAccessKey="$4"
    awsBucketName="$5"
    awsRegion="$6"

    storageClass="REDUCED_REDUNDANCY"

    date=$(date -u +"%Y%m%d")
    amzDate=$(date -u +%Y%m%dT%H%M%SZ)

    #Generate AWS request
    httpReq='PUT'
    authType='AWS4-HMAC-SHA256'
    service="s3"
    baseUrl=".${service}.amazonaws.com"

    #Set contentType field
    if hash file 2>/dev/null; then
        contentType="$(file -b --mime-type "${fileLocal}")"
    else
        contentType='application/octet-stream'
    fi

    #Calculate the payload hash
    if [ -f "${fileLocal}" ]; then
        payloadHash=$(openssl dgst -sha256 -hex < "${fileLocal}" 2>/dev/null | sed 's/^.* //')
    else
        echo "File not found: '${fileLocal}'"
        exit 1
    fi

    #Generate the canonicalRequets
    dateKey=$(hmac_sha256 key:"AWS4$awsSecretAccessKey" $date)
    dateRegionKey=$(hmac_sha256 hexkey:$dateKey $awsRegion)
    dateRegionServiceKey=$(hmac_sha256 hexkey:$dateRegionKey ${service})
    signingKey=$(hmac_sha256 hexkey:$dateRegionServiceKey "aws4_request")

    headerList='content-type;host;x-amz-content-sha256;x-amz-date;x-amz-server-side-encryption;x-amz-storage-class'

    canonicalRequest="\
${httpReq}
/${fileRemote}

content-type:${contentType}
host:${awsBucketName}${baseUrl}
x-amz-content-sha256:${payloadHash}
x-amz-date:${amzDate}
x-amz-server-side-encryption:AES256
x-amz-storage-class:${storageClass}

${headerList}
${payloadHash}"

    #Generate the canonical request hash
    canonicalRequestHash=$(printf '%s' "${canonicalRequest}" | openssl dgst -sha256 -hex 2>/dev/null | sed 's/^.* //')

    #Generate the signature
    stringToSign="\
${authType}
${amzDate}
${date}/${awsRegion}/${service}/aws4_request
${canonicalRequestHash}"

    signature=$(hmac_sha256 "hexkey:${signingKey}" "${stringToSign}")

    #Execute the request
    curl -s -L --proto-redir =https -X "${httpReq}" -T "${fileLocal}" \
    -H "Content-Type: ${contentType}" \
    -H "Host: ${awsBucketName}${baseUrl}" \
    -H "X-Amz-Content-SHA256: ${payloadHash}" \
    -H "X-Amz-Date: ${amzDate}" \
    -H "X-Amz-Server-Side-Encryption: AES256" \
    -H "X-Amz-Storage-Class: ${storageClass}" \
    -H "Authorization: ${authType} Credential=${awsAccessKeyID}/${date}/${awsRegion}/${service}/aws4_request, SignedHeaders=${headerList}, Signature=${signature}" \
    "https://${awsBucketName}${baseUrl}/${fileRemote}"
}

# Trap a signal and call the provided function
# Parameter 1: The signal number (ie: 15)
# Parameter 2: The function to call
# Extra parameters are passed as parameters of the function.
trap_with_arg() {
    func="$1" ; shift
    sig=$1
    trap "$func $*" "$sig"
}

# This function traps a signal and upload the file in a AWS S3 bucket
# Parameter 1: The signal to use while killed the child process
# Parameter 2: The PID of the child process
# Parameter 3: The file name in the AWS s3 bucket
# Parameter 4: The location path of the file to upload

func_trap() {
    # Get the AWS information
    getAWSParams
    trap=$1
    pid="$2"
    fileName="$3"
    filePath="$4"
    awsAccessKeyID=$AWS_ACCESS_KEY_ID
    awsSecretAccessKey=$AWS_SECRET_ACCESS_KEY
    awsBucketName=$AWS_BUCKET_NAME
    awsBucketFolder=$AWS_BUCKET_FOLDER
    awsRegion=$AWS_REGION

    echo "AWS_BUCKET_NAME="$awsBucketName
    echo "AWS_BUCKET_FOLDER="$awsBucketFolder
    echo "AWS_REGION="$awsRegion

    #Generate the coverage data
    echo "Save coverage data... "$filePath
    kill -$trap $pid
    wait_data $filePath
    cat $filePath
    if [ -n "$awsBucketName" ]; then
       #Upload the coverage data
       remoteFile=$fileName
       if [ -n "$awsBucketFolder" ]; then
          remoteFile=$awsBucketFolder/$fileName
       fi
       put_aws $filePath $remoteFile $awsAccessKeyID $awsSecretAccessKey $awsBucketName $awsRegion
    fi
}

# This function wait if the coverage data are posted in the POD.
wait_data() {
   n="10"
    while [ $n != 0 ]; do
        if [ -f $1 ]; then
            break
        fi
        echo "Coverage data not posted yet..."$1
        sleep 5
        n=$[$n-1]
    done
}
