# Copyright Contributors to the Open Cluster Management project

FROM openshift/origin-release:golang-1.13 as builder

ENV REMOTE_SOURCE='.'
ENV REMOTE_SOURCE_DIR='/remote-source'

ENV USE_VENDORIZED_BUILD_HARNESS=true
ENV COMPONENT_NAME=rcm-controller
ENV COMPONENT_VERSION="${COMPONENT_VERSION}"
ENV COMPONENT_TAG_EXTENSION=" "

COPY $REMOTE_SOURCE $REMOTE_SOURCE_DIR/app/
WORKDIR $REMOTE_SOURCE_DIR/app
RUN git config --global url."https://${GITHUB_TOKEN}:x-oauth-basic@github.com/".insteadOf "https://github.com/"
RUN GOFLAGS="" go build ./cmd/manager
RUN GOFLAGS="" go test -covermode=atomic -coverpkg=github.com/open-cluster-management/managedcluster-import-controller/pkg/... -c -tags testrunmain ./cmd/manager -o manager-coverage

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
RUN microdnf update

ENV REMOTE_SOURCE_DIR='/remote-source'

ENV OPERATOR=/usr/local/bin/rcm-controller \
    USER_UID=1001 \
    USER_NAME=rcm-controller \
    KLUSTERLET_CRD_FILE=/usr/local/resources/agent.open-cluster-management.io_v1beta1_klusterlet_crd.yaml

# install operator binary
COPY --from=builder $REMOTE_SOURCE_DIR/app/manager ${OPERATOR}
COPY --from=builder $REMOTE_SOURCE_DIR/app/manager-coverage ${OPERATOR}-coverage
COPY --from=builder $REMOTE_SOURCE_DIR/app/build/bin /usr/local/bin
COPY --from=builder $REMOTE_SOURCE_DIR/app/build/resources /usr/local/resources
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
