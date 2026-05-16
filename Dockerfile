FROM golang:1.26 AS build

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN go build -buildvcs=false -o /out/deepsleep ./cmd/deepsleep

FROM debian:stable-slim

WORKDIR /app
COPY --from=build /out/deepsleep /app/deepsleep
COPY config.json /app/config.json
COPY data /app/data
COPY web /app/web

ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/app/deepsleep"]
CMD ["-addr", ":8080", "-config", "/app/config.json", "-slop", "/app/data/slop.json", "-index", "/app/web/index.html"]
