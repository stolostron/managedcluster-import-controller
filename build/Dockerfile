# Copyright Contributors to the Open Cluster Management project

FROM registry.ci.openshift.org/stolostron/builder:go1.18-linux AS builder

ENV REMOTE_SOURCE='.'
ENV REMOTE_SOURCE_DIR='/remote-source'

COPY $REMOTE_SOURCE $REMOTE_SOURCE_DIR/app/
WORKDIR $REMOTE_SOURCE_DIR/app
RUN BUILD_OUTPUT_DIR=${REMOTE_SOURCE_DIR}/app make build
RUN BUILD_OUTPUT_DIR=${REMOTE_SOURCE_DIR}/app make build-coverage

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
RUN microdnf update
ENV REMOTE_SOURCE_DIR='/remote-source'

ENV OPERATOR=/usr/local/bin/managedcluster-import-controller \
    USER_UID=1001 \
    USER_NAME=managedcluster-import-controller

# install operator binary
COPY --from=builder $REMOTE_SOURCE_DIR/app/manager ${OPERATOR}
COPY --from=builder $REMOTE_SOURCE_DIR/app/manager-coverage ${OPERATOR}-coverage
COPY --from=builder $REMOTE_SOURCE_DIR/app/build/bin /usr/local/bin

RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
