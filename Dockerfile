# syntax=docker/dockerfile:1

# Use distroless as minimal base image to package the agent binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot

ARG TARGETOS
ARG TARGETARCH

COPY ${TARGETOS}/${TARGETARCH}/agent /agent

USER 65532:65532

ENTRYPOINT ["/agent"]
