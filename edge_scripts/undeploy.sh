#!/bin/bash

#Remove xDotAdapter from monit
sed -i '/xDotAdapter.pid/{N;N;N;N;d}' /etc/monitrc

#restart monit
/etc/init.d/monit restart

#Remove the init.d script
rm /etc/init.d/xDotAdapter

#Remove the default variables file
rm /etc/default/xDotAdapter

#Remove xDotAdapter from the startup script
update-rc.d -f xDotAdapter remove

#Remove the binary
rm /usr/bin/xDotAdapter
