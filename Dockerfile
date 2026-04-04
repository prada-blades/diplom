FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/diplom .

FROM alpine:3.21

WORKDIR /app

COPY --from=build /out/diplom ./diplom

EXPOSE 8080

ENTRYPOINT ["./diplom"]
CMD ["server"]
