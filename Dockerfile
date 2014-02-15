from stackbrew/ubuntu:13.04
maintainer Shipyard Project "http://shipyard-project.com"
run apt-get update
run apt-get install -y libdevmapper1.02.1 libsqlite3-0
add shipyard-agent /usr/local/bin/shipyard-agent
entrypoint ["/usr/local/bin/shipyard-agent"]
