# Shipyard Agent
This is the Shipyard Agent.  It goes on your Docker hosts.

The Shipyard Agent will gather Docker information (containers, images, etc.) from the local Docker and push it to a Shipyard instance.

# Installation
Visit the [Releases](https://github.com/shipyard/shipyard-agent/releases) page for the latest release.  Download the binary and install to your Docker host.  For example:

```
curl curl https://github.com/shipyard/shipyard-agent/releases/download/<release>/shipyard-agent -L -o /usr/local/bin/shipyard-agent
chmod +x /usr/local/bin/shipyard-agent
```

# Usage
You first need to register with your Shipyard instance.  You can do this via:

`./shipyard-agent -url http://myshipyardhost -register`

It will output an "agent key".  You will then need to authorize the host in 
Shipyard.  Login to your Shipyard instance and select "Hosts".  Click on the 
action menu for the host and select "Authorize Host".

Once authorized, you can start the agent:

`./shipyard-agent -url http://myshipyardhost -key 1234567890qwertyuiop`

