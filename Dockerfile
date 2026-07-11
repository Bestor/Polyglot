FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates sqlite
WORKDIR /app

COPY --from=build /out/server ./server

VOLUME ["/app/pb_data"]
ENV PORT=8090 PB_DATA_DIR=/app/pb_data

EXPOSE 8090
ENTRYPOINT ["/app/server"]
