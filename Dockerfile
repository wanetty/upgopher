FROM docker.io/golang:1.16-alpine AS build
WORKDIR /src
COPY go.mod  ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o upgopher .

FROM alpine:latest

WORKDIR /app
COPY --from=build /src/upgopher .
COPY static ./static
RUN mkdir uploads
EXPOSE 9090
CMD ["./upgopher"]