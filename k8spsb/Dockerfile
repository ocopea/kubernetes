# Start from a Debian image with the latest version of Go installed
# and a workspace (GOPATH) configured at /go.
FROM golang
MAINTAINER Amit Lieberman<shpandrak@gmail.com>

# Copy the local package files to the container's workspace.
ADD . /go/src/ocopea/kubernetes/k8spsb

# Build command inside the container.
# (You may fetch or manage dependencies here,
# either manually or with a tool like "godep".)
RUN go install ocopea/kubernetes/k8spsb

ENTRYPOINT /go/bin/k8spsb

EXPOSE 8000