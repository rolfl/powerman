## Button Monitor and Power Manager

This program monitors a GPIO pin on the Raspberry pi, and when that pin is activatedf for a specified, duration it runs a command

The initial purpose of the program was to monitor a shutdown button.

It also pulses an LED on a different GPIO in order to have a visible activity LED.

Useage is like:

    powerman -led 16 -button 26 -debounce 2s -- sudo shutdown -h now

The above will set up a flashing LED on port 16, listen for a button on port 26, and if the button is depressed for 2 seconds, will run the command `sudo shutdown -h now`.

When running the specified command the LED changes flashing style from a slow flash to a rapid flash.

I have this set up as a systemd service in my Raspberry pi with the following unit (`powerman.service`):

	[Unit]
	Description=Power Button Monitoring Service
	Documentation=https://github.com/rolfl/powerman
	
	[Service]
	ExecStart=/usr/local/bin/powerman -led 16 -button 26 -debounce 2s -- sudo shutdown -h now
	ExecStop=/bin/echo Kill powerman manually with kill $MAINPID
	KillMode=none
	
	[Install]
	WantedBy=multi-user.target

Note that it is intentional to ignore the systemd kill so that the fast-flashing LED lasts until the actual poweroff event.

You reconfigure systemd to accept the service with:

	systemctl daemon-reload
	systemctl enable powerman
	systemctl start powerman
	systemctl status powerman
	
Now, with the above service, when the system starts up and boots, the LED will start flashing, indicating that things are normal. Pressing the button for 2 seconds will trigger a "clean" system shutdown with a fast-flashing LED, until shutdown state is reached, at which point the LED turns off completely (indicating it is safe to unplug the system)