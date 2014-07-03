# Shipyard Agent
This is the Shipyard Agent.  It goes on your Docker hosts.

The Shipyard Agent will gather Docker information (containers, images, etc.) from the local Docker and push it to a Shipyard instance.

# Installation
The recommended way to run the agent is with Docker.

## Docker
```
docker run -i -t -v /var/run/docker.sock:/docker.sock \
  -e IP=`ifconfig eth0 | grep 'inet addr:' | cut -d: -f2 | awk '{print $1;}'` \
  -e URL=http://<shipyard-host>:8000 -p 4500:4500 shipyard/agent
```

Replace `<shipyard-host>` with your Shipyard host.

## Standalone
You can also run standalone:

Visit the [Releases](https://github.com/shipyard/shipyard-agent/releases) page for the latest release.  Download the binary and install to your Docker host.  For example:

```
curl https://github.com/shipyard/shipyard-agent/releases/download/<release>/shipyard-agent -L -o /usr/local/bin/shipyard-agent
chmod +x /usr/local/bin/shipyard-agent
```

# Usage
The first time you run the agent you must register it with Shipyard.  You can combine this for the first run and it will register automatically:

`./shipyard-agent -url http://myshipyardhost:shipyardport -register`

You will then need to authorize the host in
Shipyard.  Login to your Shipyard instance and select "Hosts".  Click on the
action menu for the host and select "Authorize Host".

Subsequent agent runs just need the key:

`./shipyard-agent -url http://myshipyardhost:shipyardport -key 1234567890qwertyuiop`
