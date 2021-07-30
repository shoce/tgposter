FROM golang:1.21.0 as build
WORKDIR /root/
RUN mkdir -p /root/tgzeposter/
COPY tgzeposter.go go.mod go.sum /root/tgzeposter/
WORKDIR /root/tgzeposter/
RUN go version
RUN go get -a -u -v
RUN ls -l -a
RUN go build -o tgzeposter tgzeposter.go
RUN ls -l -a


FROM alpine:3.18.0
RUN apk add --no-cache tzdata
RUN apk add --no-cache gcompat && ln -s -f -v ld-linux-x86-64.so.2 /lib/libresolv.so.2
RUN mkdir -p /opt/tgzeposter/
COPY A.Book.of.Days.text A.Course.in.Miracles.text /opt/tgzeposter/
COPY --from=build /root/tgzeposter/tgzeposter /opt/tgzeposter/tgzeposter
RUN ls -l -a /opt/tgzeposter/
WORKDIR /opt/tgzeposter/
ENTRYPOINT ["./tgzeposter"]

