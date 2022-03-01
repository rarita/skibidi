# syntax=docker/dockerfile:1

FROM golang:1.16-alpine
WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY *.go ./
COPY ./data /app/data

RUN go build -o /skibidi

CMD [ "/skibidi" ]
