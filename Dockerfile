FROM debian:stretch
RUN apt-get update && apt-get install -qqy glusterfs-client && rm -rf /var/lib/apt/lists/*
ADD content /
RUN mkdir -p /var/lib/docker-volumes/_glusterfs

# Add Tini
ENV TINI_VERSION v0.16.1
ADD https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini /tini
RUN chmod +x /tini
ENTRYPOINT ["/tini", "--","/docker-volume-glusterfs"]

