FROM registry.access.redhat.com/ubi8/ubi-minimal:8.1-398

ARG VCS_REF
ARG VCS_URL
ARG IMAGE_NAME
ARG IMAGE_DESCRIPTION
ARG ARCH_TYPE

ENV OPERATOR=/usr/local/bin/rcm-controller \
    USER_UID=1001 \
    USER_NAME=rcm-controller \
    ENDPOINT_CRD_FILE=/usr/local/resources/multicloud_v1beta1_endpoint_crd.yaml

# install operator binary
COPY build/_output/bin/rcm-controller ${OPERATOR}

COPY build/bin /usr/local/bin
COPY build/resources /usr/local/resources
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
