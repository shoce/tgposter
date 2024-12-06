
# https://hub.docker.com/_/golang/tags
FROM golang:1.23.4 as build
WORKDIR /root/
RUN mkdir -p /root/tgposter/
COPY tgposter.go go.mod go.sum /root/tgposter/
WORKDIR /root/tgposter/
RUN go version
RUN go get -v
RUN ls -l -a
RUN go build -o tgposter tgposter.go
RUN ls -l -a


# https://hub.docker.com/_/alpine/tags
FROM alpine:3.20.3
RUN apk add --no-cache tzdata
RUN apk add --no-cache gcompat && ln -s -f -v ld-linux-x86-64.so.2 /lib/libresolv.so.2
RUN mkdir -p /opt/tgposter/
COPY A.Book.of.Days.text A.Course.in.Miracles.text /opt/tgposter/
RUN ls -l -a /opt/tgposter/
COPY --from=build /root/tgposter/tgposter /bin/tgposter
WORKDIR /opt/tgposter/
ENTRYPOINT ["/bin/tgposter"]

