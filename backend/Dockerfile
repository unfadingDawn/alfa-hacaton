# Сборка трёх утилит backend: api (сервер), importer (загрузка данных кейса),
# llmcheck (проверка подключения к модели одним запросом).
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/importer ./cmd/importer \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/llmcheck ./cmd/llmcheck \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/eval ./cmd/eval

FROM alpine:3.20
# ca-certificates — для HTTPS к RSS и API модели, tzdata — для APP_TIMEZONE.
RUN apk add --no-cache ca-certificates tzdata wget \
 && adduser -D -u 10001 app
COPY --from=build /out/ /usr/local/bin/
USER app
WORKDIR /app
EXPOSE 8080
CMD ["api"]
