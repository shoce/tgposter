
ARG APPNAME=tgposter

# https://hub.docker.com/_/golang/tags
FROM golang:1.25.0 AS build
ARG APPNAME
ENV APPNAME=$APPNAME
ENV CGO_ENABLED=0
RUN mkdir -p /src/$APPNAME/
COPY *.go go.mod go.sum /src/$APPNAME/
WORKDIR /src/$APPNAME/
RUN go version
RUN go get -v
RUN ls -l -a
RUN go build -o $APPNAME .
RUN ls -l -a


# https://hub.docker.com/_/alpine/tags
FROM alpine:3.22.1
ARG APPNAME
ENV APPNAME=$APPNAME
RUN apk add --no-cache tzdata
RUN apk add --no-cache gcompat && ln -s -f -v ld-linux-x86-64.so.2 /lib/libresolv.so.2
RUN mkdir -p /opt/$APPNAME/
COPY *.text /opt/$APPNAME/
RUN ls -l -a /opt/$APPNAME/
COPY --from=build /src/$APPNAME/$APPNAME /bin/$APPNAME
WORKDIR /opt/$APPNAME/
ENTRYPOINT /bin/$APPNAME

