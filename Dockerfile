FROM golang:latest

ENV GOPATH=/go
RUN go get github.com/FiloSottile/gvt

ADD . /go/src/github.com/travis-ci/cloud-brain
WORKDIR /go/src/github.com/travis-ci/cloud-brain

#RUN gvt rebuild


RUN rm bin/*
RUN make
ARG DOCKER_BUILD_BIN
RUN make bin/"$DOCKER_BUILD_BIN"
