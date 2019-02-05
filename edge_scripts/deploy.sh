#!/bin/bash

#Copy binary to /usr/local/bin
mv xDotAdapter /usr/bin

#Ensure binary is executable
chmod +x /usr/bin/xDotAdapter

#Set up init.d resources so that xDotAdapter is started when the gateway starts
mv xDotAdapter.etc.initd /etc/init.d/xDotAdapter
mv xDotAdapter.etc.default /etc/default/xDotAdapter

#Ensure init.d script is executable
chmod +x /etc/init.d/xDotAdapter

#Add adapter to log rotate
cat << EOF > /etc/logrotate.d/xDotAdapter.conf
/var/log/xDotAdapter {
    size 10M
    rotate 3
    compress
    copytruncate
    missingok
}
EOF

#Remove xDotAdapter from monit in case it was already there
sed -i '/xDotAdapter.pid/{N;N;N;N;d}' /etc/monitrc

#Add the adapter to monit
sed -i '/#  check process apache with pidfile/i \
  check process xDotAdapter with pidfile \/var\/run\/xDotAdapter.pid \
    start program = "\/etc\/init.d\/xDotAdapter start" with timeout 60 seconds \
    stop program  = "\/etc\/init.d\/xDotAdapter stop" \
    depends on edge \
 ' /etc/monitrc

#reload monit config
monit reload

#Start the adapter
monit start xDotAdapter

echo "xDotAdapter Deployed"
