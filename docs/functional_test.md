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
1. Submit your branch changes in a draft PR.  Once travis is done building, you can reference the image.
2. Lookup the image postfix for rcm-controller image and export it.  This is the suffix value after the version (e.g. `2.2.0`).  If you are pulling the image from quay, the list of images is at https://quay.io/repository/open-cluster-management/rcm-controller?tab=tags:
   ```
    export COMPONENT_TAG_EXTENSION=-SNAPSHOT-2020-04-01-20-49-00
   ```
3. Set some environment variables for Git access:
   ```
   export GITHUB_USER=<GITHUB_USER>
   export GITHUB_TOKEN=<GITHUB_TOKEN>
   ```   
4. Set some environment variables for quay.io docker access:
   ```
   export DOCKER_USER=<Docker username>
   export DOCKER_PASS=<Docker password>
   ```   
5. Make sure you have permission to `docker pull` the rcm-controller image
6. Run the following command to setup & start a kind cluster:
   ```
    make component/test/functional
   ```
   *NOTE* If you get the error:
   ```
   ERROR: node(s) already exist for a cluster with the name "functional-test"
   ```
   run the following command to cleanup a previous kind cluster:
   ```
  make kind-delete-cluster
   ```   
