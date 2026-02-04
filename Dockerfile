# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot@sha256:f9f84bd968430d7d35e8e6d55c40efb0b980829ec42920a49e60e65eac0d83fc
ARG TARGETARCH
WORKDIR /
COPY bin/manager-linux.${TARGETARCH} /manager
USER 65532:65532

ENTRYPOINT ["/manager"]