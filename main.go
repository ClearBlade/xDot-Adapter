package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"xDot-Adapter/xDotSerial"

	cb "github.com/clearblade/Go-SDK"
	mqttTypes "github.com/clearblade/mqtt_parsing"
	mqtt "github.com/clearblade/paho.mqtt.golang"
	"github.com/hashicorp/logutils"
)

const (
	platURL                   = "http://localhost:9000"
	messURL                   = "localhost:1883"
	msgSubscribeQos           = 0
	msgPublishQos             = 0
	serialRead                = "receive"
	serialWrite               = "send"
	MTSIO_CMD                 = "mts-io-sysfs"
	CONDUIT_PRODUCT_ID_PREFIX = "MTCDT"
	XDOT_PRODUCT_ID           = "MTAC-MFSER-DTE"
)

var (
	platformURL         string //Defaults to http://localhost:9000
	messagingURL        string //Defaults to localhost:1883
	sysKey              string
	sysSec              string
	deviceName          string //Defaults to xDotSerialAdapter
	activeKey           string
	logLevel            string //Defaults to info
	initLoRaWANPublic   bool
	adapterConfigCollID string
	readInterval        int
	isReading           bool
	isWriting           bool

	serialPortName = ""

	// peer 2 peer mode adapter setting defaults
	networkAddress        = "00:11:22:33"
	networkSessionKey     = "00:11:22:33:00:11:22:33:00:11:22:33:00:11:22:33"
	networkDataKey        = "33:22:11:00:33:22:11:00:33:22:11:00:33:22:11:00"
	transmissionDataRate  = "DR8"
	transmissionFrequency = "915500000"

	// public ota lora mode adapter setting defaults (empty strings are required adapter settings)
	networkID        = ""
	networkKey       = ""
	frequencySubBand = "0"

	topicRoot = "wayside/lora"

	serialPort          *xDotSerial.XdotSerialPort
	serialConfigChanged = false
	cbBroker            cbPlatformBroker
	cbSubscribeChannel  <-chan *mqttTypes.Publish
	endWorkersChannel   chan string
)

type cbPlatformBroker struct {
	name         string
	clientID     string
	client       *cb.DeviceClient
	platformURL  *string
	messagingURL *string
	systemKey    *string
	systemSecret *string
	username     *string
	password     *string
	topic        string
	qos          int
}

func init() {
	flag.StringVar(&sysKey, "systemKey", "", "system key (required)")
	flag.StringVar(&sysSec, "systemSecret", "", "system secret (required)")
	flag.StringVar(&deviceName, "deviceName", "xDotSerialAdapter", "name of device (optional)")
	flag.StringVar(&activeKey, "password", "", "password (or active key) for device authentication (required)")
	flag.StringVar(&platformURL, "platformURL", platURL, "platform url (optional)")
	flag.StringVar(&messagingURL, "messagingURL", messURL, "messaging URL (optional)")
	flag.StringVar(&logLevel, "logLevel", "info", "The level of logging to use. Available levels are 'debug, 'info', 'warn', 'error', 'fatal' (optional)")
	flag.IntVar(&readInterval, "readInterval", 10, "The number of seconds to wait before each successive serial port read. (optional)")
	flag.BoolVar(&initLoRaWANPublic, "initLoRaWANPublic", false, "Initialize xdot card to use LoRaWAN Public (optional - peer to peer mode is default)")
	flag.StringVar(&adapterConfigCollID, "adapterConfigCollectionID", "", "The ID of the data collection used to house adapter configuration (required)")
}

func usage() {
	log.Printf("Usage: xDotAdapter [options]\n\n")
	flag.PrintDefaults()
}

func validateFlags() {
	flag.Parse()

	if sysKey == "" || sysSec == "" || deviceName == "" || activeKey == "" {

		log.Printf("ERROR - Missing required flags\n\n")
		flag.Usage()
		os.Exit(1)
	}
}

func main() {
	fmt.Println("Starting xDotAdapter...")

	//Validate the command line flags
	flag.Usage = usage
	validateFlags()

	//create the log file with the correct permissions
	logfile, err := os.OpenFile("/var/log/xDotAdapter", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}

	defer logfile.Close()

	//Initialize the logging mechanism
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"},
		MinLevel: logutils.LogLevel(strings.ToUpper(logLevel)),
		Writer:   logfile,
	}

	log.SetOutput(filter)

	cbBroker = cbPlatformBroker{

		name:         "ClearBlade",
		clientID:     deviceName + "client",
		client:       nil,
		platformURL:  &platformURL,
		messagingURL: &messagingURL,
		systemKey:    &sysKey,
		systemSecret: &sysSec,
		username:     &deviceName,
		password:     &activeKey,
		topic:        "wayside/serial/#",
		qos:          msgSubscribeQos,
	}

	// Initialize ClearBlade Client
	if err = initCbClient(cbBroker); err != nil {
		log.Println(err.Error())
		log.Println("Unable to initialize CB broker client. Exiting.")
		return
	}

	defer close(endWorkersChannel)
	endWorkersChannel = make(chan string)

	//Handle OS interrupts to shut down gracefully
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	sig := <-c

	log.Printf("[INFO] OS signal %s received, ending go routines.", sig)

	//stop serial data mode when adapter is killed
	log.Println("[INFO] Stopping Serial Data Mode...")
	if err := serialPort.StopSerialDataMode(); err != nil {
		log.Println("[WARN] initCbClient - Error stopping serial data mode: " + err.Error())
	}

	//End the existing goRoutines
	endWorkersChannel <- "Stop Channel"
	endWorkersChannel <- "Stop Channel"
	os.Exit(0)
}

// ClearBlade Client init helper
func initCbClient(platformBroker cbPlatformBroker) error {
	log.Println("[INFO] initCbClient - Initializing the ClearBlade client")

	log.Printf("[DEBUG] initCbClient - Platform URL: %s\n", *(platformBroker.platformURL))
	log.Printf("[DEBUG] initCbClient - Platform Messaging URL: %s\n", *(platformBroker.messagingURL))
	log.Printf("[DEBUG] initCbClient - System Key: %s\n", *(platformBroker.systemKey))
	log.Printf("[DEBUG] initCbClient - System Secrent: %s\n", *(platformBroker.systemSecret))
	log.Printf("[DEBUG] initCbClient - Username: %s\n", *(platformBroker.username))
	log.Printf("[DEBUG] initCbClient - Password: %s\n", *(platformBroker.password))

	cbBroker.client = cb.NewDeviceClientWithAddrs(*(platformBroker.platformURL), *(platformBroker.messagingURL), *(platformBroker.systemKey), *(platformBroker.systemSecret), *(platformBroker.username), *(platformBroker.password))

	for err := cbBroker.client.Authenticate(); err != nil; {
		log.Printf("[ERROR] initCbClient - Error authenticating %s: %s\n", platformBroker.name, err.Error())
		log.Println("[ERROR] initCbClient - Will retry in 1 minute...")

		// sleep 1 minute
		time.Sleep(time.Duration(time.Minute * 1))
		err = cbBroker.client.Authenticate()
	}

	//Retrieve adapter configuration data
	log.Println("[INFO] initCbClient - Retrieving adapter configuration...")
	adapter_settings := getAdapterConfig()

	if serialPortName == "" {
		log.Println("[DEBUG] initCbClient - Retrieving serial port name")
		setSerialPortName(adapter_settings)

		if serialPortName == "" {
			log.Fatalf("[FATAL] initCbClient - Unable to detect the serial port xDot is using")
			return errors.New("Unable to detect the serial port xDot is using")
		}
	}

	serialPort = xDotSerial.CreateXdotSerialPort(serialPortName, 115200, time.Millisecond*2500)

	log.Println("[DEBUG] initCbClient - Opening serial port")
	if err := serialPort.OpenSerialPort(); err != nil {
		log.Panic("[FATAL] initCbClient - Error opening serial port: " + err.Error())
	}

	//Turn off serial data mode in case it is currently on
	if err := serialPort.StopSerialDataMode(); err != nil {
		log.Println("[WARN] initCbClient - Error stopping serial data mode: " + err.Error())
	}

	//Initialize xDot network settings and data rate
	if initLoRaWANPublic {
		log.Println("[INFO] initCbClient - Configuring xDot for LoRaWAN Public")
		initXDotLoRaWANPublic()
	} else {
		log.Println("[INFO] initCbClient - Configuring xDot for Peer To Peer")
		initXDotPeerToPeer()
	}

	log.Println("[INFO] initCbClient - Initializing MQTT")
	callbacks := cb.Callbacks{OnConnectionLostCallback: OnConnectLost, OnConnectCallback: OnConnect}
	if err := cbBroker.client.InitializeMQTTWithCallback(platformBroker.clientID, "", 30, nil, nil, &callbacks); err != nil {
		log.Fatalf("[FATAL] initCbClient - Unable to initialize MQTT connection with %s: %s", platformBroker.name, err.Error())
		return err
	}

	return nil
}

//If the connection to the broker is lost, we need to reconnect and
//re-establish all of the subscriptions
func OnConnectLost(client mqtt.Client, connerr error) {
	log.Printf("[INFO] OnConnectLost - Connection to broker was lost: %s\n", connerr.Error())

	//End the existing goRoutines
	endWorkersChannel <- "Stop Channel"
	endWorkersChannel <- "Stop Channel"

	//We don't need to worry about manally re-initializing the mqtt client. The auto reconnect logic will
	//automatically try and reconnect. The reconnect interval could be as much as 20 minutes.
}

//When the connection to the broker is complete, set up the subscriptions
func OnConnect(client mqtt.Client) {
	log.Println("[INFO] OnConnect - Connected to ClearBlade Platform MQTT broker")

	//CleanSession, by default, is set to true. This results in non-durable subscriptions.
	//We therefore need to re-subscribe
	log.Println("[DEBUG] OnConnect - Begin Configuring Subscription(s)")

	var err error
	for cbSubscribeChannel, err = subscribe(topicRoot + "/+/request"); err != nil; {
		//Wait 30 seconds and retry
		log.Printf("[ERROR] OnConnect - Error subscribing to MQTT: %s\n", err.Error())
		log.Println("[ERROR] OnConnect - Will retry in 30 seconds...")
		time.Sleep(time.Duration(30 * time.Second))
		cbSubscribeChannel, err = subscribe(topicRoot + "/#")
	}

	isReading = false
	isWriting = false

	//Start subscribe worker
	go subscribeWorker()

	//Start read loop
	go readWorker()
}

func initXDotPeerToPeer() {
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
	log.Println("[INFO] initXDotPeerToPeer - Setting network join mode...")
	if valueChanged, err := serialPort.SetNetworkJoinMode(xDotSerial.PeerToPeerMode); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set the device class to class C
	log.Println("[INFO] initXDotPeerToPeer - Setting device class...")
	if valueChanged, err := serialPort.SetDeviceClass(xDotSerial.DeviceClassC); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set network address
	log.Println("[INFO] initXDotPeerToPeer - Setting network address...")
	if valueChanged, err := serialPort.SetNetworkAddress(networkAddress); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set network session key
	log.Println("[INFO] initXDotPeerToPeer - Setting network session key...")
	if valueChanged, err := serialPort.SetNetworkSessionKey(networkSessionKey); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set data session key
	log.Println("[INFO] initXDotPeerToPeer - Setting data session key...")
	if valueChanged, err := serialPort.SetDataSessionKey(networkDataKey); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set transmission data rate
	log.Println("[INFO] initXDotPeerToPeer - Setting transmission data rate...")
	if valueChanged, err := serialPort.SetDataRate(transmissionDataRate); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set transmission frequency
	log.Println("[INFO] initXDotPeerToPeer - Setting transmission frequency...")
	if valueChanged, err := serialPort.SetFrequency(transmissionFrequency); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	if serialConfigChanged == true {
		log.Println("[DEBUG] initXDotPeerToPeer - xDot configuration changed, saving new values...")
		//Save the xDot configuration
		if err := serialPort.SaveConfiguration(); err != nil {
			log.Println("[WARN] initXDotPeerToPeer - Error saving xDot configuration: " + err.Error())
		}
		// Temporarily comment this out as it appear this hangs the xDot
		//
		// else {
		// 	//Reset the xDot CPU
		// 	log.Println("[DEBUG] initXDotPeerToPeer - Resetting xDot CPU...")
		// 	if err := serialPort.ResetXDotCPU(); err != nil {
		// 		log.Panic("[FATAL] initXDotPeerToPeer - Error resetting xDot CPU: " + err.Error())
		// 	}
		// }
	}
}

func initXDotLoRaWANPublic() {
	// AT+NJM=1
	// AT+NI=00-11-22-33-44-aa-bb-cc (from lora network server, all connecting lora device suse same)
	// AT+NK=00.11.22.33.44.55.66.77.88.99.aa.bb.cc.dd.ee.ff (from lora network server, all connecting lora device suse same)
	// AT+FSB=1 (based off lora network server, should come from config collection)
	// save it!
	// join it!

	//Set network join mode to LoRaWAN Public
	log.Println("[INFO] initXDotLoRaWANPublic - Setting network join mode...")
	if valueChanged, err := serialPort.SetNetworkJoinMode(xDotSerial.OtaJoinMode); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set the device class to class C
	log.Println("[INFO] initXDotLoRaWANPublic - Setting device class...")
	if valueChanged, err := serialPort.SetDeviceClass(xDotSerial.DeviceClassC); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set Network ID
	log.Println("[INFO] initXDotLoRaWANPublic - Setting Network ID...")
	if valueChanged, err := serialPort.SetNetworkID(networkID); err != nil {
		panic(err.Error())
	} else {
		if valueChanged {
			serialConfigChanged = true
		}
	}

	//Set Network Key
	log.Println("[INFO] initXDotLoRaWANPublic - Setting Network Key...")
	if valueChanged, err := serialPort.SetNetworkKey(networkKey); err != nil {
		panic(err.Error())
	} else {
		if valueChanged {
			serialConfigChanged = true
		}
	}

	//Set Network Frequency Sub-Band
	log.Println("[INFO] initXDotLoRaWANPublic - Setting Network Frequence Sub-Band...")
	if valueChanged, err := serialPort.SetFrequencySubBand(frequencySubBand); err != nil {
		panic(err.Error())
	} else {
		if valueChanged {
			serialConfigChanged = true
		}
	}

	if serialConfigChanged == true {
		log.Println("[DEBUG] initXDotLoRaWANPublic - xDot configuration changed, saving new values...")
		//Save the xDot configuration
		if err := serialPort.SaveConfiguration(); err != nil {
			log.Println("[WARN] initXDotLoRaWANPublic - Error saving xDot configuration: " + err.Error())
		}
	}

	//Join network
	log.Println("[INFO] initXDotLoRaWANPublic - Joining Network...")
	if err := serialPort.JoinNetwork(); err != nil {
		panic(err.Error())
	}

	// For us to start receiving downlinks, we need to AT+SEND 2 empty messages...
	//  http://www.multitech.net/developer/software/lora/class-c-walkthrough/
	// 1. AT+SEND to acknowledge Join Accept.
	// 2. AT+SEND to acknowledge first downlink MAC commands
	log.Println("[INFO] initXDotLoRaWANPublic - AT+SEND to ack Join Accept")
	if err := serialPort.SendData(""); err != nil {
		panic(err.Error())
	}

	log.Println("[INFO] initXDotLoRaWANPublic - AT+SEND to ack first downlink MAC commands")
	if err := serialPort.SendData(""); err != nil {
		panic(err.Error())
	}

}

func subscribeWorker() {
	log.Println("[INFO] subscribeWorker - Starting subscribeWorker")

	//Flush serial port one last time
	if err := serialPort.FlushSerialPort(); err != nil {
		log.Println("[ERROR] subscribeWorker - Error flushing serial port: " + err.Error())
	}

	defer serialPort.StopSerialDataMode()
	//Enter serial data mode
	log.Println("[DEBUG] subscribeWorker - Entering serial data mode...")
	if err := serialPort.StartSerialDataMode(); err != nil {
		log.Panic("[FATAL] subscribeWorker - Error starting serial data mode: " + err.Error())
	}

	//Wait for subscriptions to be received
	for {
		select {
		case message, ok := <-cbSubscribeChannel:
			if ok {
				//Determine if a read or write request was received
				if strings.HasSuffix(message.Topic.Whole, serialRead+"/request") {
					log.Println("[INFO] subscribeWorker - Handling read request...")
					readFromSerialPort()
				} else if strings.HasSuffix(message.Topic.Whole, serialWrite+"/request") {
					// If write request...
					log.Println("[INFO] subscribeWorker - Handling write request...")
					writeToSerialPort(string(message.Payload))
				} else {
					log.Printf("[DEBUG] subscribeWorker - Unknown request received: topic = %s, payload = %#v\n", message.Topic.Whole, message.Payload)
				}
			}
		case _ = <-endWorkersChannel:
			//End the current go routine when the stop signal is received
			log.Println("[INFO] subscribeWorker - Stopping subscribeWorker")
			return
		}
	}
}

func readWorker() {
	log.Println("[INFO] readWorker - Starting readWorker")
	ticker := time.NewTicker(time.Duration(readInterval) * time.Second)

	for {
		select {
		case <-ticker.C:
			log.Println("[DEBUG] readWorker - Reading from serial port")
			readFromSerialPort()
		case <-endWorkersChannel:
			log.Println("[DEBUG] readWorker - stopping ticker")
			ticker.Stop()
			return
		}
	}
}

// Subscribes to a topic
func subscribe(topic string) (<-chan *mqttTypes.Publish, error) {
	log.Printf("[DEBUG] subscribe - Subscribing to topic %s\n", topic)
	subscription, error := cbBroker.client.Subscribe(topic, cbBroker.qos)
	if error != nil {
		log.Printf("[ERROR] subscribe - Unable to subscribe to topic: %s due to error: %s\n", topic, error.Error())
		return nil, error
	}

	log.Printf("[DEBUG] subscribe - Successfully subscribed to = %s\n", topic)
	return subscription, nil
}

// Publishes data to a topic
func publish(topic string, data string) error {
	log.Printf("[DEBUG] publish - Publishing to topic %s\n", topic)
	error := cbBroker.client.Publish(topic, []byte(data), cbBroker.qos)
	if error != nil {
		log.Printf("[ERROR] publish - Unable to publish to topic: %s due to error: %s\n", topic, error.Error())
		return error
	}

	log.Printf("[DEBUG] publish - Successfully published message to = %s\n", topic)
	return nil
}

func getAdapterConfig() map[string]interface{} {
	var settingsJson map[string]interface{}

	log.Println("[INFO] getAdapterConfig - Retrieving adapter config")

	//Retrieve the adapter configuration row
	query := cb.NewQuery()
	query.EqualTo("adapter_name", "xDotSerialPortAdapter")

	//A nil query results in all rows being returned
	log.Println("[DEBUG] getAdapterConfig - Executing query against table " + adapterConfigCollID)
	results, err := cbBroker.client.GetData(adapterConfigCollID, query)
	if err != nil {
		log.Println("[DEBUG] getAdapterConfig - Adapter configuration could not be retrieved. Using defaults")
		log.Printf("[DEBUG] getAdapterConfig - Error: %s\n", err.Error())
	} else {
		if len(results["DATA"].([]interface{})) > 0 {
			log.Printf("[DEBUG] getAdapterConfig - Adapter config retrieved: %#v\n", results)
			log.Println("[INFO] getAdapterConfig - Adapter config retrieved")

			//topic root
			if results["DATA"].([]interface{})[0].(map[string]interface{})["topic_root"] != nil {
				log.Printf("[DEBUG] getAdapterConfig - Setting topicRoot to %s\n", results["DATA"].([]interface{})[0].(map[string]interface{})["topic_root"].(string))
				topicRoot = results["DATA"].([]interface{})[0].(map[string]interface{})["topic_root"].(string)
			} else {
				log.Printf("[DEBUG] getAdapterConfig - Topic root is nil. Using default value %s\n", topicRoot)
			}

			//adapter_settings
			log.Println("[DEBUG] getAdapterConfig - Retrieving adapter settings...")
			if results["DATA"].([]interface{})[0].(map[string]interface{})["adapter_settings"] != nil {
				if err := json.Unmarshal([]byte(results["DATA"].([]interface{})[0].(map[string]interface{})["adapter_settings"].(string)), &settingsJson); err != nil {
					log.Printf("[DEBUG] getAdapterConfig - Error while unmarshalling json: %s. Defaulting all adapter settings.\n", err.Error())
				}
			} else {
				log.Println("[DEBUG] applyAdapterConfig - Settings are nil. Defaulting all adapter settings.")
			}
		} else {
			log.Println("[DEBUG] getAdapterConfig - No rows returned. Using defaults")
		}
	}

	if settingsJson == nil {
		settingsJson = make(map[string]interface{})
	}

	applyAdapterSettings(settingsJson)

	return settingsJson
}

func applyAdapterSettings(adapterSettings map[string]interface{}) {

	if initLoRaWANPublic {
		//networkID
		if adapterSettings["networkID"] != nil {
			networkID = adapterSettings["networkID"].(string)
			log.Printf("[DEBUG] applyAdapterConfig - Setting networkID to %s", networkID)
		} else {
			log.Printf("[ERROR] applyAdapterSettings - A networkID value is expected, using a bogus value that likely will not work\n")
			adapterSettings["networkID"] = networkID
		}
		//networkKey
		if adapterSettings["networkKey"] != nil {
			networkKey = adapterSettings["networkKey"].(string)
			log.Printf("[DEBUG] applyAdapterConfig - Setting networkKey to %s", networkKey)
		} else {
			log.Printf("[ERROR] applyAdapterSettings - A networkKey value is expected, using a bogus value that likely will not work\n")
			adapterSettings["networkKey"] = networkKey
		}
		//frequencySubBand
		if adapterSettings["frequencySubBand"] != nil {
			frequencySubBand = adapterSettings["frequencySubBand"].(string)
			log.Printf("[DEBUG] applyAdapterConfig - Setting frequencySubBand to %s", frequencySubBand)
		} else {
			adapterSettings["frequencySubBand"] = frequencySubBand
		}
	} else {
		//networkAddress
		if adapterSettings["networkAddress"] != nil {
			log.Printf("[DEBUG] applyAdapterConfig - Setting networkAddress to %s", adapterSettings["networkAddress"].(string)+"\n")
			networkAddress = adapterSettings["networkAddress"].(string)
		} else {
			adapterSettings["networkAddress"] = networkAddress
		}

		//networkSessionKey
		if adapterSettings["networkSessionKey"] != nil {
			log.Printf("[DEBUG] applyAdapterConfig - Setting networkSessionKey to %s", adapterSettings["networkSessionKey"].(string)+"\n")
			networkSessionKey = adapterSettings["networkSessionKey"].(string)
		} else {
			adapterSettings["networkSessionKey"] = networkSessionKey
		}

		//networkDataKey
		if adapterSettings["networkDataKey"] != nil {
			log.Printf("[DEBUG] applyAdapterConfig - Setting networkDataKey to %s", adapterSettings["networkDataKey"].(string)+"\n")
			networkDataKey = adapterSettings["networkDataKey"].(string)
		} else {
			adapterSettings["networkDataKey"] = networkDataKey
		}

		//transmissionDataRate
		if adapterSettings["transmissionDataRate"] != nil {
			log.Printf("[DEBUG] applyAdapterConfig - Setting transmissionDataRate to %s", adapterSettings["transmissionDataRate"].(string)+"\n")
			transmissionDataRate = adapterSettings["transmissionDataRate"].(string)
		} else {
			adapterSettings["transmissionDataRate"] = transmissionDataRate
		}

		//transmissionFrequency
		if adapterSettings["transmissionFrequency"] != nil {
			log.Printf("[DEBUG] applyAdapterConfig - Setting transmissionFrequency to %s", adapterSettings["transmissionFrequency"].(string)+"\n")
			transmissionFrequency = adapterSettings["transmissionFrequency"].(string)
		} else {
			adapterSettings["transmissionFrequency"] = transmissionFrequency
		}
	}

}

func setSerialPortName(adapterSettings map[string]interface{}) {
	// 1. Determine if we are running on a Multitech Conduit
	// 2. Determine which port (ap1 or ap2) has a product ID of MTAC-MFSER-DTE

	productId := getProductId("")
	if strings.Contains(productId, CONDUIT_PRODUCT_ID_PREFIX) {
		log.Printf("[INFO] setSerialPortName - Multitech Conduit detected: %s\n", productId)
		//We are running on a Multitech Conduit. Use ap1 and ap2 as port names
		portProductID := getProductId("ap1")
		if strings.Contains(portProductID, XDOT_PRODUCT_ID) {
			log.Printf("[INFO] setSerialPortName - XDOT detected on ap1\n")
			serialPortName = "/dev/ttyAP1"
		} else {
			portProductID := getProductId("ap2")
			if strings.Contains(portProductID, XDOT_PRODUCT_ID) {
				log.Printf("[INFO] setSerialPortName - XDOT detected on ap2\n")
				serialPortName = "/dev/ttyAP2"
			} else {
				log.Printf("[ERROR] setSerialPortName - XDOT not detected on ap1 or ap2\n")
			}
		}
	} else {
		log.Printf("[ERROR] setSerialPortName - Not running on a multitech conduit\n")
		//Must be an actual xDot we are running on, not sure what port name
		//to use at this time
		serialPortName = ""
	}
}

func getProductId(portName string) string {
	port := ""
	if portName != "" {
		port += portName + "/"
	}
	port += "product-id"

	return executeMtsIoCommand(port)
}

func executeMtsIoCommand(mtsioObject string) string {
	cmd := exec.Command(MTSIO_CMD, "show", mtsioObject)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		log.Printf("[ERROR] executeMtsIoCommand - ERROR executing mts-io-sysfs command: %s\n", err.Error())
		return ""
	} else {
		log.Printf("[DEBUG] executeMtsIoCommand - Command response received: %s\n", out.String())
		return out.String()
	}
}

func readFromSerialPort() {
	// 1. Read all data from serial port
	// 2. Publish data to platform as string
	var data string

	for isWriting {
		log.Println("[INFO] readFromSerialPort - Currently writing to serial port. Waiting 1 second...")
		time.Sleep(1 * time.Second)
	}

	isReading = true
	buffer, err := serialPort.ReadSerialPort()
	for err == nil {
		data += buffer
		buffer, err = serialPort.ReadSerialPort()
	}

	isReading = false

	if err != nil && !strings.Contains(err.Error(), "EOF") {
		log.Printf("[ERROR] readFromSerialPort - ERROR reading from serial port: %s\n", err.Error())
	} else {
		log.Printf("[DEBUG] readFromSerialPort - Data read from serial port: %s\n", data)

		if data != "" {
			//If there are any slashes in the data, we need to escape them so duktape
			//doesn't throw a SyntaxError: unterminated string (line 1) error
			data = strings.Replace(data, `\`, `\\`, -1)

			//Publish data to message broker
			log.Println("[INFO] readFromSerialPort - Data read from serial port: " + data)
			err := publish(topicRoot+"/"+serialRead+"/response", data)
			if err != nil {
				log.Printf("[ERROR] readFromSerialPort - ERROR publishing to topic: %s\n", err.Error())
			}
		} else {
			log.Println("[DEBUG] readFromSerialPort - No data read from serial port, skipping publish.")
		}
	}
}

func writeToSerialPort(payload string) {
	for isReading {
		log.Println("[INFO] writeToSerialPort - Currently reading from serial port. Waiting 1 second...")
		time.Sleep(1 * time.Second)
	}

	log.Printf("[INFO] writeToSerialPort - Writing to serial port: %s\n", payload)
	isWriting = true
	err := serialPort.WriteSerialPort(string(payload))
	isWriting = false
	if err != nil {
		log.Printf("[ERROR] writeToSerialPort - ERROR writing to serial port: %s\n", err.Error())
	}
}
