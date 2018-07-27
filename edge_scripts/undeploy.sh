#!/bin/bash

#Remove xDotAdapter from monit
sed -i '/xDotAdapter.pid/{N;N;N;N;d}' /etc/monitrc

#Remove the init.d script
rm /etc/init.d/xDotAdapter

#Remove the default variables file
rm /etc/default/xDotAdapter

#Remove the binary
rm /usr/bin/xDotAdapter

#reload monit config
monit reload