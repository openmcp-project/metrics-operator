# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot@sha256:2b7c93f6d6648c11f0e80a48558c8f77885eb0445213b8e69a6a0d7c89fc6ae4
ARG TARGETARCH
WORKDIR /
COPY bin/manager-linux.${TARGETARCH} /manager
USER 65532:65532

ENTRYPOINT ["/manager"]