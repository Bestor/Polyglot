FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/polyglot ./cmd/polyglot
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/valorantapi ./cmd/valorantapi
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/mcpserver ./cmd/mcpserver
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/discordbot ./cmd/discordbot
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/cachewarmer ./cmd/cachewarmer
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/chesscomapi ./cmd/chesscomapi
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/chesscomwarmer ./cmd/chesscomwarmer

FROM alpine:3.20
RUN apk add --no-cache ca-certificates sqlite
WORKDIR /app

COPY --from=build /out/polyglot ./polyglot
COPY --from=build /out/valorantapi ./valorantapi
COPY --from=build /out/mcpserver ./mcpserver
COPY --from=build /out/discordbot ./discordbot
COPY --from=build /out/cachewarmer ./cachewarmer
COPY --from=build /out/chesscomapi ./chesscomapi
COPY --from=build /out/chesscomwarmer ./chesscomwarmer
COPY openapi ./openapi
COPY cmd/cachewarmer/players.txt ./players.txt
COPY cmd/chesscomwarmer/usernames.txt ./usernames.txt

VOLUME ["/app/pb_data"]
ENV PORT=8090 PB_DATA_DIR=/app/pb_data

EXPOSE 8091 8092 8093 8094
ENTRYPOINT ["/app/polyglot"]
