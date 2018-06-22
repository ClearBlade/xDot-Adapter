package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"
	"xDotAdapter/xDotSerial"

	"github.com/hashicorp/logutils"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

var (
	logLevel string //Defaults to info

	serialPortName        string //Defaults to /dev/ttyAP1
	networkAddress        string //Defaults to 00:11:22:33
	networkSessionKey     string //Defaults to 00:11:22:33:00:11:22:33:00:11:22:33:00:11:22:33
	networkDataKey        string //Defaults to 33:22:11:00:33:22:11:00:33:22:11:00:33:22:11:00
	transmissionDataRate  string //Defaults to DR8
	transmissionFrequency string //Defaults to 915500000

	serialPort          *xDotSerial.XdotSerialPort
	serialConfigChanged = false

	workerChannel    chan string
	endWorkerChannel chan string

	isReading = false
	isWriting = false

	deviceEUI string
)

func init() {
	flag.StringVar(&serialPortName, "serialPortName", "/dev/ttyAP1", "serial port path (optional)")
	flag.StringVar(&networkAddress, "networkAddress", "00:11:22:33", "network address (optional)")
	flag.StringVar(&networkSessionKey, "networkSessionKey", "00:11:22:33:00:11:22:33:00:11:22:33:00:11:22:33", "network session key (optional)")
	flag.StringVar(&networkDataKey, "networkDataKey", "33:22:11:00:33:22:11:00:33:22:11:00:33:22:11:00", "network data key (optional)")
	flag.StringVar(&transmissionDataRate, "transmissionDataRate", "DR8", "transmission data rate (optional)")
	flag.StringVar(&transmissionFrequency, "transmissionFrequency", "915500000", "transmission frequency (optional)")
	flag.StringVar(&logLevel, "logLevel", "info", "The level of logging to use. Available levels are 'debug, 'info', 'warn', 'error', 'fatal' (optional)")
}

func usage() {
	log.Printf("Usage: xDotAdapter [options]\n\n")
	flag.PrintDefaults()
}

func validateFlags() {
	flag.Parse()
}

func main() {
	fmt.Println("Starting xDotAdapter...")

	//Validate the command line flags
	flag.Usage = usage
	validateFlags()

	//Initialize the logging mechanism
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"},
		MinLevel: logutils.LogLevel(strings.ToUpper(logLevel)),
		Writer: &lumberjack.Logger{
			Filename:   "/var/log/xDotAdapter",
			MaxSize:    10, // megabytes
			MaxBackups: 5,
			MaxAge:     28, //days
		},
	}
	log.SetOutput(filter)

	serialPort = xDotSerial.CreateXdotSerialPort(serialPortName, 115200, time.Millisecond*2500)

	log.Println("[INFO] main - Opening serial port")
	if err := serialPort.OpenSerialPort(); err != nil {
		log.Panic("[FATAL] main - Error opening serial port: " + err.Error())
	}

	//Turn off serial data mode in case it is currently on
	if err := serialPort.StopSerialDataMode(); err != nil {
		log.Println("[WARN] main - Error stopping serial data mode: " + err.Error())
	}

	configureXDot()

	retrieveDeviceId()

	//Flush serial port one last time
	if err := serialPort.FlushSerialPort(); err != nil {
		log.Println("[ERROR] main - Error flushing serial port: " + err.Error())
	}

	defer serialPort.StopSerialDataMode()
	defer serialPort.CloseSerialPort()
	//Enter serial data mode
	log.Println("[INFO] main - Entering serial data mode...")
	if err := serialPort.StartSerialDataMode(); err != nil {
		log.Panic("[FATAL] main - Error starting serial data mode: " + err.Error())
	}

	workerChannel = make(chan string)
	endWorkerChannel = make(chan string)

	defer close(workerChannel)
	defer close(endWorkerChannel)

	log.Println("[INFO] main - Starting serialWorker")
	go serialWorker()

	//Endless loop to read/write from serial port
	for {
		//Read serial port
		workerChannel <- ""

		//Write serial port
		workerChannel <- "01" + deviceEUI + time.Now().UTC().Format(time.RFC3339)
		time.Sleep(1 * time.Minute)
	}
}

func configureXDot() {
	//http://www.multitech.net/developer/software/mdot-software/peer-to-peer/

	// In order to get the xDot card in Peer to Peer mode, we need to write AT commands
	// to the serial port:
	//
	// AT+NJM=3 --> Set network join mode to peer to peer (3)
	// AT+NA=00:11:22:33 --> Set network address: Must be the same for all xDots. devAddr in LoraMac
	// AT+NSK=00:11:22:33:00:11:22:33:00:11:22:33:00:11:22:33 --> Set network session key: Must be the same for all xDots.
	// AT+DSK=33:22:11:00:33:22:11:00:33:22:11:00:33:22:11:00 --> Set data session key: Must be the same for all xDots.
	// AT+TXDR=DR8 (US:DR8-DR13,EU:DR0-DR6) --> Set the transmission data rate for all channels
	// AT+TXF=915500000 (US-ONLY:915.5-919.7) --> Set the transmission frequency
	// AT&W --> Save configuration to flash memory
	// ATZ --> Reset CPU: Takes about 3 seconds
	// AT+SD --> Serial Data Mode

	//Set network join mode to peer to peer
	log.Printf("[INFO] configureXDot - Setting network join mode to %s\n", xDotSerial.PeerToPeerMode)
	if valueChanged, err := serialPort.SetNetworkJoinMode(xDotSerial.PeerToPeerMode); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set the device class to class C
	log.Printf("[INFO] configureXDot - Setting device class to %s\n", xDotSerial.DeviceClassC)
	if valueChanged, err := serialPort.SetDeviceClass(xDotSerial.DeviceClassC); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set network address
	log.Printf("[INFO] configureXDot - Setting network address to %s\n", networkAddress)
	if valueChanged, err := serialPort.SetNetworkAddress(networkAddress); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set network session key
	log.Printf("[INFO] configureXDot - Setting network session key to %s\n", networkSessionKey)
	if valueChanged, err := serialPort.SetNetworkSessionKey(networkSessionKey); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set data session key
	log.Printf("[INFO] configureXDot - Setting data session key to %s\n", networkDataKey)
	if valueChanged, err := serialPort.SetDataSessionKey(networkDataKey); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set transmission data rate
	log.Printf("[INFO] configureXDot - Setting transmission data rate to %s\n", transmissionDataRate)
	if valueChanged, err := serialPort.SetDataRate(transmissionDataRate); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set transmission frequency
	log.Printf("[INFO] configureXDot - Setting transmission frequency to %s\n", transmissionFrequency)
	if valueChanged, err := serialPort.SetFrequency(transmissionFrequency); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	if serialConfigChanged == true {
		log.Println("[INFO] configureXDot - xDot configuration changed, saving new values...")
		//Save the xDot configuration
		if err := serialPort.SaveConfiguration(); err != nil {
			log.Println("[WARN] configureXDot - Error saving xDot configuration: " + err.Error())
		} else {
			//Reset the xDot CPU
			log.Println("[INFO] configureXDot - Resetting xDot CPU...")
			if err := serialPort.ResetXDotCPU(); err != nil {
				log.Panic("[FATAL] configureXDot - Error resetting xDot CPU: " + err.Error())
			}
		}
	}
}

func serialWorker() {
	//Wait for read requests to be received
	for {
		select {
		case packet, ok := <-workerChannel:
			if ok {
				if packet == "" {
					log.Println("[INFO] serialWorker - Read request received")

					var data string

					for isWriting {
						log.Println("[INFO] serialWorker - Currently writing to serial port. Waiting 1 second...")
						time.Sleep(1 * time.Second)
					}

					isReading = true
					buffer, err := serialPort.ReadSerialPort(false)
					for err == nil {
						data += buffer
						buffer, err = serialPort.ReadSerialPort(false)
					}

					isReading = false

					if err != nil && !strings.Contains(err.Error(), "EOF") {
						log.Printf("[ERROR] serialWorker - ERROR reading from serial port: %s\n", err.Error())
					} else {
						log.Printf("[INFO] serialWorker - Data read from serial port: %s\n", data)

						if data == "" {
							log.Println("[INFO] serialWorker - No data read from serial port, skipping publish.")
						}
					}
				} else {
					log.Printf("[INFO] serialWorker - Write request received: %s\n", packet)
					// Write string to serial port
					for isReading {
						log.Println("[INFO] serialWorker - Currently reading from serial port. Waiting 1 second...")
						time.Sleep(1 * time.Second)
					}
					log.Printf("[INFO] serialWorker - Writing to serial port: %s\n", packet)
					isWriting = true
					err := serialPort.WriteSerialPort(string(packet))
					isWriting = false
					if err != nil {
						log.Printf("[ERROR] serialWorker - ERROR writing to serial port: %s\n", err.Error())
					}
				}
			}
		case _ = <-endWorkerChannel:
			//End the current go routine when the stop signal is received
			log.Println("[INFO] serialWorker - Stopping serialWorker")
			return
		}
	}
}

func retrieveDeviceId() {
	deviceID, err := serialPort.GetDeviceID()
	if err != nil {
		log.Printf("[ERROR] retrieveDeviceId - ERROR retrieving device ID: %s\n", err.Error())
	}
	log.Printf("[INFO] retrieveDeviceId - Retrieved device ID: %s\n", deviceID)

	deviceEUI = deviceID
}
