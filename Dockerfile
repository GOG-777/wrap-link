FROM golang:1.22-alpine
WORKDIR /app
COPY go.mod ./
COPY pkg ./pkg
COPY api ./api
COPY cmd ./cmd
COPY public ./public
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/wrap ./cmd/wrap \
    && adduser -D -u 10001 wrapuser \
    && mkdir -p /app/data \
    && chown -R wrapuser:wrapuser /app
USER wrapuser
EXPOSE 8080
ENTRYPOINT ["/app/wrap"]
