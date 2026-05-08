FROM golang:1.23-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/twitch-irc .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates \
	&& adduser -D -H -u 10001 appuser

WORKDIR /app

COPY --from=build /out/twitch-irc /usr/local/bin/twitch-irc

USER appuser

EXPOSE 2112

ENTRYPOINT ["twitch-irc"]
CMD ["-config", "/app/config.toml"]
