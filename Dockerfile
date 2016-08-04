FROM golang:latest

ADD . /go/src/github.com/travis-ci/cloud-brain

WORKDIR /go/src/github.com/travis-ci/cloud-brain

ENV GOPATH=/go

RUN go get github.com/FiloSottile/gvt 
RUN gvt rebuild
RUN make
RUN make bin
