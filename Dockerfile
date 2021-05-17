FROM golang:1.16-alpine as build

WORKDIR /app
COPY . /app
RUN go build -o dist/gha_get_token

FROM alpine:3.13
COPY --from=build /app/dist/gha_get_token /usr/local/bin/gha_get_token
RUN chmod +x /usr/local/bin/gha_get_token

CMD ["gha_get_token"]