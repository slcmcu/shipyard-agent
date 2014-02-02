# Shipyard Agent
This is the Shipyard Agent.

The Shipyard Agent will gather Docker information (containers, images, etc.) from the local Docker and push it to a Shipyard instance.

# Usage
Run this in a Docker container (note the `-v` to bind mount the Docker socket for the agent):

`docker run -i -t -v /var/run/docker.sock:/docker.sock shipyard/agent -url http://<shipyard-host>:<shipyard-port> -docker /docker.sock -register`

This will cause the Agent container to register and run.  In Shipyard, click on the 
action menu for the host and select "Authorize Host" to enable.

If you wish to restart the agent, simply omit the `-register` and use the key from the output of the first container with `-key`.

