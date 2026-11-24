# Build the local gateway/mocks used by the T8 Compose topology.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/gateway ./cmd/gateway \
 && CGO_ENABLED=0 go build -trimpath -o /out/legacy-mock ./cmd/legacy-mock \
 && CGO_ENABLED=0 go build -trimpath -o /out/policy ./cmd/policy \
 && CGO_ENABLED=0 go build -trimpath -o /out/controlplane ./cmd/controlplane

# Alpine is used instead of gcr.io/distroless because gcr.io is unreachable in
# this environment. The binaries are CGO-disabled static binaries.
FROM alpine:3.20
RUN adduser -D -H -u 65532 nonroot
COPY --from=build /out/gateway /gateway
COPY --from=build /out/legacy-mock /legacy-mock
COPY --from=build /out/policy /policy
COPY --from=build /out/controlplane /controlplane
COPY config/policy.example.json /config/policy.example.json
USER nonroot:nonroot
ENTRYPOINT ["/gateway"]