# build
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd/ cmd/
COPY motor/ motor/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /motor ./cmd/motor

# imagem final: só o binário estático e as regras
FROM scratch
COPY --from=build /motor /app/motor
COPY regras/ /app/regras/
ENTRYPOINT ["/app/motor"]
