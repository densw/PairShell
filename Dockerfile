FROM golang:1.27-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath -o /pairshelld ./cmd/pairshelld
FROM alpine:3.22
RUN addgroup -g 10001 pairshell && adduser -D -u 10001 -G pairshell pairshell && mkdir /data && chown pairshell /data
USER 10001:10001
COPY --from=build /pairshelld /usr/local/bin/pairshelld
WORKDIR /data
ENTRYPOINT ["pairshelld"]
CMD ["--listen","0.0.0.0:8443","--tls-cert","/certs/fullchain.pem","--tls-key","/certs/privkey.pem","--witness-dir","/data/receipts","--witness-key","/data/witness.key"]
