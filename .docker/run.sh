#!/bin/bash
IP=${IP:-`ifconfig eth0 | grep 'inet addr:' | cut -d: -f2 | awk '{print $1;}'`}
SHIPYARD_URL=${URL:-http://172.17.42.1:8000}
KEY=$(/usr/local/bin/shipyard-agent -ip $IP -url $SHIPYARD_URL -register 2>&1 | tail -1 | sed 's/.*Key: //g' | tr -d ' ')
API_VERSION=${API_VERSION:-v1.9}

/usr/local/bin/shipyard-agent -ip $IP -url $SHIPYARD_URL -key $KEY -docker /docker.sock -api-version $API_VERSION
