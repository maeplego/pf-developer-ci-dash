FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -o /ci-dash ./cmd/ci-dash

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /ci-dash /ci-dash
USER nobody
ENV CI_DASH_HTTP_ADDR=:8115
EXPOSE 8115
ENTRYPOINT ["/ci-dash"]
