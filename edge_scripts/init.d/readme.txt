Instructions for use:

1. Copy xDotAdapter.etc.default file into /etc/default, name the file "xDotAdapter"
2. Copy xDotAdapter.etc.initd file into /etc/init.d, name the file "xDotAdapter"
3. From a terminal prompt, execute the following commands:
	3a. chmod 755 /etc/init.d/xDotAdapter
	3b. chown root:root /etc/init.d/xDotAdapter
	3c. update-rc.d xDotAdapter defaults 85

If you wish to start the adapter, rather than reboot, issue the following command from a terminal prompt:

	/etc/init.d/xDotAdapter start