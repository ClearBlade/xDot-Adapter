#!/bin/bash

#stop the xDotAdapter service
/etc/init.d/xDotAdapter stop

#Remove the init.d script
rm /etc/init.d/xDotAdapter

#Remove xDotAdapter from the startup script
update-rc.d -f xDotAdapter remove

#Remove the binary
rm /usr/bin/xDotAdapter

#Remove all other artifacts
rm -rf ./xDotAdapter