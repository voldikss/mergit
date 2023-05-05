FROM golang:1.19 AS builder
WORKDIR /build

COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://goproxy.cn,direct
RUN go mod download

COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o mergit

FROM alpine
WORKDIR /app

ENV TZ Asia/Shanghai

COPY --from=builder /build/mergit /app/mergit

ENTRYPOINT [ "/app/mergit" ]
