FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN go build -o reliabilityops-api ./cmd/api

FROM alpine:3.20

WORKDIR /app

COPY --from=builder /app/reliabilityops-api .

EXPOSE 8080

CMD ["./reliabilityops-api"]
