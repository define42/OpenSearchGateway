FROM golang:1.26.1-alpine AS build

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/opensearchgateway .

FROM alpine:3.22

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=build /out/opensearchgateway /usr/local/bin/opensearchgateway

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/opensearchgateway"]
