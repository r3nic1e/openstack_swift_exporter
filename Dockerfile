FROM golang:1.10

WORKDIR /go/src/github.com/r3nic1e/openstack_swift_exporter/
ADD *.go .

ARG SHA1
ARG TAG
ARG DATE

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags  " -X main.VERSION=$TAG -X main.COMMIT_SHA1=$SHA1 -X main.BUILD_DATE=$DATE " -a -installsuffix cgo -o swift_exporter .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /bin/
COPY --from=0 /go/src/github.com/r3nic1e/openstack_swift_exporter/ .

EXPOSE     9500
ENTRYPOINT [ "/bin/swift_exporter" ]
