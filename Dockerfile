FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /dasyra ./cmd/dasyra

# pdftotext reads PDFs, ddgr is the search fallback when SearXNG is down; jq is built in (gojq)
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata poppler-utils python3 py3-pip \
    && pip3 install --break-system-packages --no-cache-dir ddgr
ENV DB_PATH=/data/db.sqlite LISTEN=:8080
COPY --from=build /dasyra /usr/local/bin/dasyra
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["dasyra"]
CMD ["serve"]
