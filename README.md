# Docker volume plugin for GlusterFS

This plugin uses GlusterFS as distributed data storage for containers.

Unlike original implementations, it works in **Swarm mode**.
It uses the new (1.13+) **managed plugin subsystem** https://docs.docker.com/engine/extend/.

## Installation

The plugin requires an API server on your Gluster bricks.
Use https://github.com/aravindavk/glusterfs-rest instructions for manual installation.
Ansible installation for the API can be found here : <TODO>

Then on your Docker nodes:
```
docker plugin install hjdr4plugins/docker-volume-glusterfs servers=srv1:srv2 [parameter=value]
```
where srv1 and srv2 are Gluster bricks you want to use.
You can put from 1 to as many servers you want.
The driver will try to reach the API for every node in the order your provide until someone answers, or throw an error if no server is reachable. 

Valid parameters:defaults are :
- servers:"" (this is **required**) 
- login:docker
- password:docker
- port:9000
- base:/var/lib/gluster/bricks

## Usage

This plugin is capable of creating and removing volumes.
```
docker volume create --driver=hjdr4plugins/docker-volume-glusterfs myGlusterVolume
```

```
docker run --rm --volume-driver=hjdr4plugins/docker-volume-glusterfs --volume myGlusterVolume:/data alpine touch /data/helo
```

## Building your own version

You will need `go`, `make`, `sudo` and `docker`.

- fork this project and go into its directory
- set env variables : `export VERSION="<yourVersion>";export IMAGE="<yourImage>"; export SERVERS="<yourServers>"`
- make a clean install : `make clean plugin plugin-install`
- do your tests
- push the plugin to the registry: `docker plugin push <yourImage>`

See https://docs.docker.com/engine/extend/#developing-a-plugin for complete instructions.

***remark***: the registry you use must NOT have regular images, Docker needs a dedicated plugin repository. If you use DockerHub, just create another account.

## LICENSE

MIT
