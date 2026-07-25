FROM node:22-alpine AS web

WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web ./
RUN npm run build

FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/pulse ./cmd/pulse

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/pulse /pulse
COPY --from=web /src/web/dist /web
ENV PULSE_WEB_DIR=/web
EXPOSE 8080
ENTRYPOINT ["/pulse"]
