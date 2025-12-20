#!/bin/bash

# Copy api to apipublic
rm -rf lib/apipublic

mkdir -p lib/apipublic

cp -r api/v1alpha1 lib/apipublic/v1alpha1

rm -rf lib/apipublic/v1alpha1/validation.go # this file import the internal/api/v1alpha1/validation.go, need to remove it

echo "Successfully copied api to lib/apipublic"

# Copy reqid to lib
rm -rf lib/reqid

cp -r pkg/reqid lib/reqid

echo "Successfully copied reqid to lib"

# Replace github.com/flightctl/flightctl/api/v1alpha1 to github.com/flightctl/flightctl/lib/apipublic/v1alpha1 in apipublic/v1alpha1/agent/spec.gen.cfg and apipublic/v1alpha1/agent/types.gen.cfg
sed -i '' 's/github.com\/flightctl\/flightctl\/api\/v1alpha1/github.com\/flightctl\/flightctl\/lib\/apipublic\/v1alpha1/g' lib/apipublic/v1alpha1/agent/spec.gen.cfg
sed -i '' 's/github.com\/flightctl\/flightctl\/api\/v1alpha1/github.com\/flightctl\/flightctl\/lib\/apipublic\/v1alpha1/g' lib/apipublic/v1alpha1/agent/types.gen.cfg

# Rerun go generate
go generate ./lib/apipublic/v1alpha1/agent/docs.go

echo "Successfully generated api docs"

go generate ./lib/api/client/docs.go

echo "Successfully generated client docs"
