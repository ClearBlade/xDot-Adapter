#!/bin/bash

#Copy binary to /usr/local/bin
mv xDotAdapter /usr/bin

#Ensure binary is executable
chmod +x /usr/bin/xDotAdapter

#Set up init.d resources so that xDotAdapter is started when the gateway starts
cp xDotAdapter.etc.initd /etc/init.d/xDotAdapter
cp xDotAdapter.etc.default /etc/default/xDotAdapter

#Ensure init.d script is executable
chmod +x /etc/init.d/xDotAdapter

#Add the adapter to the startup script
update-rc.d xDotAdapter defaults

echo "xDotAdapter Deployed"
