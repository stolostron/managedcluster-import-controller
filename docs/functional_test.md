[comment]: # ( Copyright Contributors to the Open Cluster Management project )

# Functional Test

### Before Testing
1. Make sure you have [ginkgo](https://onsi.github.io/ginkgo/) excutable ready in your env. If not, do the following:
   ```
    go get github.com/onsi/ginkgo/ginkgo
    go get github.com/onsi/gomega/...
   ```
2. If you want to run functional test locally with KinD, you will need to install KinD: https://kind.sigs.k8s.io/docs/user/quick-start/#installation


## Run Functional Test Against Hub Clusters

1. `oc login` to the hub cluster
2. `make functional-test`

## Run Functional Test Locally with KinD
1. Use the following command to build image & run functional tests locally:
   ```
    make functional-test-full
   ```
