FROM golang:1.13.1 AS build
WORKDIR /go/src/github.com/zricethezav/gitleaks
COPY . .
RUN GO111MODULE=on CGO_ENABLED=0 go build -o bin/gitleaks *.go

FROM alpine:3.7
RUN apk add --no-cache bash git openssh
COPY --from=build /go/src/github.com/zricethezav/gitleaks/bin/* /usr/bin/
ENTRYPOINT ["gitleaks"]

# How to use me :

# docker build -t gitleaks .
# docker run --rm --name=gitleaks gitleaks --repo=https://github.com/zricethezav/gitleaks

# This will check for secrets in https://github.com/zricethezav/gitleaks
