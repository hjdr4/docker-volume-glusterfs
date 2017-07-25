FROM debian:stretch
RUN apt-get update && apt-get install -qqy glusterfs-client && rm -rf /var/lib/apt/lists/*
ADD content /
RUN mkdir -p /var/lib/docker-volumes/_glusterfs
ENTRYPOINT ["/docker-volume-glusterfs"]
