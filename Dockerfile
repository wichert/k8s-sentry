FROM golang:1.13-alpine AS builder

WORKDIR /go/src/app
COPY . .
RUN go build

FROM alpine:3.10
COPY --from=builder /go/src/app/k8s-sentry /

CMD ["/k8s-sentry"]