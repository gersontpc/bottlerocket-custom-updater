FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/bottlerocket-updater ./cmd/updater

FROM public.ecr.aws/amazonlinux/amazonlinux:2023
COPY --from=build /out/bottlerocket-updater /usr/local/bin/bottlerocket-updater
ENTRYPOINT ["/usr/local/bin/bottlerocket-updater"]
