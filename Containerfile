# Dedicated LAN/internet server for the port's multiplayer (cmd/konung-server).
#
#   podman build -t konung-server -f Containerfile .
#   podman run -d --name konung-server -p 7777:7777/tcp -p 7777:7777/udp \
#       -v konung-data:/data konung-server
#
# /data holds the SQLite database, the LAN version's GAME.RES (copy it into
# the volume; without it players can log in but not enter the world) and,
# optionally, its SERVER file (session name and length). UDP 7777 answers LAN discovery; on the
# internet only TCP 7777 is needed (players type the address).
FROM docker.io/library/golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /konung-server ./cmd/konung-server
RUN mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /konung-server /konung-server
# The image runs as the distroless nonroot user (65532); a named volume
# takes this directory's owner, so the server can write its database.
COPY --from=build --chown=65532:65532 /data /data
VOLUME /data
EXPOSE 7777/tcp 7777/udp
ENTRYPOINT ["/konung-server", "-data", "/data"]
