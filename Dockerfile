FROM golang:1.21.5-alpine3.19 as build

WORKDIR /app
COPY . /app
RUN go build -o dist/gh_get_token

FROM alpine:3.19
COPY --from=build /app/dist/gh_get_token /usr/local/bin/gh_get_token
RUN chmod +x /usr/local/bin/gh_get_token

CMD ["gh_get_token"]