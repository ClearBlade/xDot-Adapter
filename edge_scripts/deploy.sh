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
update-rc.d xDotAdapter defaults 85

#Remove mtsIoAdapter from monit in case it was already there
sed -i '/xDotAdapter.pid/{N;N;N;N;d}' /etc/monitrc

#Add the adapter to monit
sed -i '/#  check process apache with pidfile/i \
  check process xDotAdapter with pidfile \/var\/run\/xDotAdapter.pid \
    start program = "\/etc\/init.d\/xDotAdapter start" with timeout 60 seconds \
    stop program  = "\/etc\/init.d\/xDotAdapter stop" \
    depends on edge \
 ' /etc/monitrc

#restart monit
/etc/init.d/monit restart

echo "xDotAdapter Deployed"
