FROM golang:1.21-alpine as builder

RUN apk update
RUN apk upgrade
RUN apk --no-cache add -U ca-certificates
RUN apk add --no-cache bash

WORKDIR /usr/src/app
COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY config/config.yml /usr/src/app/config.yml
COPY config/config2.yml /usr/src/app/config2.yml

COPY ./cmd/ /usr/src/app/cmd/
COPY ./internal/ /usr/src/app/internal/
COPY ./pkg/ /usr/src/app/pkg/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags '-extldflags "-static"' -o /usr/bin/app /usr/src/app/cmd/main.go

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/bin/app /usr/bin/app
COPY --from=builder /usr/src/app/config.yml /etc/aliver/config.yml
COPY --from=builder /usr/src/app/config2.yml /etc/aliver/config2.yml
EXPOSE 6223
ENTRYPOINT ["/usr/bin/app"]