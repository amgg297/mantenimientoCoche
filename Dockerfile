# Build
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/mantenimiento .

# Runtime
FROM alpine:3.22
RUN apk add --no-cache su-exec \
	&& adduser -D -H -u 1000 mantenimiento
COPY --from=build /out/mantenimiento /usr/local/bin/mantenimiento
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh /usr/local/bin/mantenimiento
ENV ADDR=0.0.0.0:8080 \
	DB_PATH=/data/mantenimiento.db
EXPOSE 8080
VOLUME /data
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
	CMD wget -qO- http://127.0.0.1:8080/ >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["mantenimiento"]
