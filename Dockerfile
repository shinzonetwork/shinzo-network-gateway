FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY ./ ./
RUN CGO_ENABLED=0 go build -o /out/gateway ./cmd/gateway

FROM cgr.dev/chainguard/static:latest
COPY --from=build /out/gateway /gateway
# TODO(tzdybal): remove once hosts are fetched from shinzohub
COPY hosts.txt /hosts.txt
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/gateway"]
CMD ["start"]
