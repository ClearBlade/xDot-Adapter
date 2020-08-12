package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"xDot-Adapter/xDotSerial"

	cb "github.com/clearblade/Go-SDK"
	mqttTypes "github.com/clearblade/mqtt_parsing"
	mqtt "github.com/clearblade/paho.mqtt.golang"
	"github.com/hashicorp/logutils"
)

const (
	platURL                        = "http://localhost:9000"
	messURL                        = "localhost:1883"
	msgSubscribeQos                = 0
	msgPublishQos                  = 0
	serialRead                     = "receive"
	serialWrite                    = "send"
	mtsIoCmd                       = "mts-io-sysfs"
	xDotUtilCmd                    = "xdot-util"
	conduitProductIDPrefix         = "MTCDT"
	xDotProductID                  = "MTAC-XDOT"
	adapterConfigCollectionDefault = "adapter_config"
)

var (
	platformURL             string //Defaults to http://localhost:9000
	messagingURL            string //Defaults to localhost:1883
	sysKey                  string
	sysSec                  string
	deviceName              string //Defaults to xDotSerialAdapter
	activeKey               string
	logLevel                string //Defaults to info
	initLoRaWANPublic       bool
	adapterConfigCollection string
	readInterval            int

	xDotPort        = ""
	serialPortName  = ""
	adapterSettings map[string]interface{}

	// peer to peer mode adapter setting defaults
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

	serialPortLock = &sync.Mutex{}
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
	flag.IntVar(&readInterval, "readInterval", 3, "The number of seconds to wait before each successive serial port read. (optional)")
	flag.BoolVar(&initLoRaWANPublic, "initLoRaWANPublic", false, "Initialize xdot card to use LoRaWAN Public (optional - peer to peer mode is default)")
	flag.StringVar(&adapterConfigCollection, "adapterConfigCollection", adapterConfigCollectionDefault, "The name of the data collection used to house adapter configuration (required)")
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

	rand.Seed(time.Now().UnixNano())

	//Initialize the logging mechanism
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"},
		MinLevel: logutils.LogLevel(strings.ToUpper(logLevel)),
		Writer:   os.Stdout,
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
	if err := initCbClient(cbBroker); err != nil {
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

	//End the existing goRoutines
	endWorkersChannel <- "Stop Channel"
	endWorkersChannel <- "Stop Channel"

	//stop serial data mode when adapter is killed
	log.Println("[INFO] Stopping Serial Data Mode...")
	if err := serialPort.StopSerialDataMode(); err != nil {
		log.Println("[WARN] initCbClient - Error stopping serial data mode: " + err.Error())
	}

	os.Exit(0)
}

// ClearBlade Client init helper
func initCbClient(platformBroker cbPlatformBroker) error {
	log.Println("[INFO] initCbClient - Initializing the ClearBlade client")

	log.Printf("[DEBUG] initCbClient - Platform URL: %s\n", *(platformBroker.platformURL))
	log.Printf("[DEBUG] initCbClient - Platform Messaging URL: %s\n", *(platformBroker.messagingURL))
	log.Printf("[DEBUG] initCbClient - System Key: %s\n", *(platformBroker.systemKey))
	log.Printf("[DEBUG] initCbClient - System Secret: %s\n", *(platformBroker.systemSecret))
	log.Printf("[DEBUG] initCbClient - Username: %s\n", *(platformBroker.username))
	log.Printf("[DEBUG] initCbClient - Password: %s\n", *(platformBroker.password))

	cbBroker.client = cb.NewDeviceClientWithAddrs(*(platformBroker.platformURL), *(platformBroker.messagingURL), *(platformBroker.systemKey), *(platformBroker.systemSecret), *(platformBroker.username), *(platformBroker.password))

	for _, err := cbBroker.client.Authenticate(); err != nil; {
		log.Printf("[ERROR] initCbClient - Error authenticating %s: %s\n", platformBroker.name, err.Error())
		log.Println("[ERROR] initCbClient - Will retry in 1 minute...")

		// sleep 1 minute
		time.Sleep(time.Duration(time.Minute * 1))
		_, err = cbBroker.client.Authenticate()
	}

	//Retrieve adapter configuration data
	log.Println("[INFO] initCbClient - Retrieving adapter configuration...")
	getAdapterConfig()

	initializeXdot()

	log.Println("[INFO] initCbClient - Initializing MQTT")
	callbacks := cb.Callbacks{OnConnectionLostCallback: OnConnectLost, OnConnectCallback: OnConnect}
	if err := cbBroker.client.InitializeMQTTWithCallback(platformBroker.clientID+"-"+strconv.Itoa(rand.Intn(10000)), "", 30, nil, nil, &callbacks); err != nil {
		log.Fatalf("[FATAL] initCbClient - Unable to initialize MQTT connection with %s: %s", platformBroker.name, err.Error())
		return err
	}

	return nil
}

func initializeXdot() {
	if serialPortName == "" {
		log.Println("[DEBUG] initCbClient - Retrieving serial port name")
		setSerialPortName()

		if serialPortName == "" {
			log.Fatalf("[FATAL] initCbClient - Unable to detect the serial port xDot is using")
		}
	}

	//We need to reset the xDot just to make sure it is in a state where we can proceed
	resetXdot()

	//Wait 3 seconds for the reset to take place
	time.Sleep(5 * time.Second)

	serialPort = xDotSerial.CreateXdotSerialPort(serialPortName, 115200, time.Millisecond*2500)

	log.Println("[DEBUG] initCbClient - Opening serial port")
	if err := serialPort.OpenSerialPort(); err != nil {
		log.Panic("[FATAL] initCbClient - Error opening serial port: " + err.Error())
	}

	//Turn off serial data mode in case it is currently on
	if err := serialPort.StopSerialDataMode(); err != nil {
		log.Panic("[FATAL] initCbClient - Error stopping serial data mode: " + err.Error())
	}

	//Initialize xDot network settings and data rate
	if initLoRaWANPublic {
		log.Println("[INFO] initCbClient - Configuring xDot for LoRaWAN Public")
		initXDotLoRaWANPublic()
	} else {
		log.Println("[INFO] initCbClient - Configuring xDot for Peer To Peer")
		initXDotPeerToPeer()
	}
}

//OnConnectLost - If the connection to the broker is lost, we need to reconnect and
//re-establish all of the subscriptions
func OnConnectLost(client mqtt.Client, connerr error) {
	log.Printf("[ERROR] OnConnectLost - Connection to broker was lost: %s\n", connerr.Error())

	//We can't rely on MQTT auto-reconnect because it is most likely that our auth token expired
	//When the connection is lost, just exit
	log.Fatalln("[FATAL] onConnectLost - MQTT Connection was lost. Stopping Adapter to force device reauth.")
}

//OnConnect - When the connection to the broker is complete, set up the subscriptions
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
		cbSubscribeChannel, err = subscribe(topicRoot + "/+/request")
	}

	//Flush serial port one last time
	log.Println("[DEBUG] OnConnect - about to flush serial port")
	if err := serialPort.FlushSerialPort(); err != nil {
		log.Println("[ERROR] OnConnect - Error flushing serial port: " + err.Error())
	}

	log.Println("[DEBUG] OnConnect - about to enter serial data mode")
	//Enter serial data mode
	log.Println("[DEBUG] OnConnect - Entering serial data mode...")
	if err := serialPort.StartSerialDataMode(); err != nil {
		log.Panic("[FATAL] OnConnect - Error starting serial data mode: " + err.Error())
	}

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

	//Set transmission frequency
	log.Println("[INFO] initXDotPeerToPeer - Setting transmission frequency...")
	if valueChanged, err := serialPort.SetFrequency(transmissionFrequency); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	initXDotCommon()

	if serialConfigChanged == true {
		log.Println("[DEBUG] initXDotPeerToPeer - xDot configuration changed, saving new values...")
		//Save the xDot configuration
		if err := serialPort.SaveConfiguration(); err != nil {
			log.Println("[WARN] initXDotPeerToPeer - Error saving xDot configuration: " + err.Error())
		} else {
			//Reset the xDot CPU
			log.Println("[DEBUG] initXDotPeerToPeer - Resetting xDot CPU...")
			if err := serialPort.ResetXDotCPU(); err != nil {
				log.Panic("[FATAL] initXDotPeerToPeer - Error resetting xDot CPU: " + err.Error())
			}
		}
	}
}

func initXDotLoRaWANPublic() {
	// AT+NJM=1
	// AT+NI=00-11-22-33-44-aa-bb-cc (from lora network server, all connecting lora device suse same)
	// AT+NK=00.11.22.33.44.55.66.77.88.99.aa.bb.cc.dd.ee.ff (from lora network server, all connecting lora device suse same)
	// AT+FSB=1 (based off lora network server, should come from config collection)
	// AT+TXDR=3 (could depend on solution, pulling from config collection)
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

	// Set public network mode to 1 AT+PN=1
	log.Println("[INFO] initXDotLoRaWANPublic - Setting public network mode...")
	if valueChanged, err := serialPort.SetPublicNetworkMode(xDotSerial.PublicLoRaWANNetworkMode); err != nil {
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

	initXDotCommon()

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

func initXDotCommon() {
	//Set transmit power
	if adapterSettings["transmitPower"] != nil {
		log.Println("[INFO] initXDotCommon - Setting transmit power...")
		if valueChanged, err := serialPort.SetTransmitPower(adapterSettings["transmitPower"].(string)); err != nil {
			panic(err.Error())
		} else {
			if valueChanged == true {
				serialConfigChanged = true
			}
		}
	}

	//Set antenna gain
	if adapterSettings["antennaGain"] != nil {
		log.Println("[INFO] initXDotCommon - Setting antenna gain...")
		if valueChanged, err := serialPort.SetAntennaGain(adapterSettings["antennaGain"].(string)); err != nil {
			panic(err.Error())
		} else {
			if valueChanged == true {
				serialConfigChanged = true
			}
		}
	}

	//Set transmission data rate
	log.Println("[INFO] initXDotCommon - Setting transmission data rate...")
	if valueChanged, err := serialPort.SetDataRate(transmissionDataRate); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}
}

func subscribeWorker() {
	log.Println("[INFO] subscribeWorker - Starting subscribeWorker")

	defer serialPort.StopSerialDataMode()

	//Wait for subscriptions to be received
	for {
		select {
		case message, ok := <-cbSubscribeChannel:
			if ok {
				//Determine if a read or write request was received
				if strings.HasSuffix(message.Topic.Whole, serialRead+"/request") {
					log.Println("[INFO] subscribeWorker - Handling read request...")
					go readFromSerialPort()
				} else if strings.HasSuffix(message.Topic.Whole, serialWrite+"/request") {
					// If write request...
					log.Printf("[INFO] subscribeWorker - Handling write request: %s\n", message.Payload)
					go writeToSerialPort(string(message.Payload))
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

	//May need to make sure the go routines are not stacking up
	//We may need to eventually add code to determine if the mutex is
	//locked prior to adding another goroutine to read
	for {
		select {
		case <-ticker.C:
			log.Println("[DEBUG] readWorker - Reading from serial port")
			go readFromSerialPort()
		case <-endWorkersChannel:
			log.Println("[DEBUG] readWorker - stopping ticker")
			ticker.Stop()
			return
		}
	}
}

// func mutexIsLocked(m *sync.Mutex) bool {
// 	state := reflect.ValueOf(m).Elem().FieldByName("state")
// 	return state.Int()&mutexLocked == mutexLocked
// }

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

func getAdapterConfig() {
	log.Println("[INFO] getAdapterConfig - Retrieving adapter config")

	//Retrieve the adapter configuration row
	query := cb.NewQuery()
	query.EqualTo("adapter_name", "xDotSerialPortAdapter")

	//A nil query results in all rows being returned
	log.Println("[DEBUG] getAdapterConfig - Executing query against table " + adapterConfigCollection)
	results, err := cbBroker.client.GetDataByName(adapterConfigCollection, query)
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
				if err := json.Unmarshal([]byte(results["DATA"].([]interface{})[0].(map[string]interface{})["adapter_settings"].(string)), &adapterSettings); err != nil {
					log.Printf("[DEBUG] getAdapterConfig - Error while unmarshalling json: %s. Defaulting all adapter settings.\n", err.Error())
				}
			} else {
				log.Println("[DEBUG] applyAdapterConfig - Settings are nil. Defaulting all adapter settings.")
			}
		} else {
			log.Println("[DEBUG] getAdapterConfig - No rows returned. Using defaults")
		}
	}

	if adapterSettings == nil {
		adapterSettings = make(map[string]interface{})
	}

	applyAdapterSettings()
}

func applyAdapterSettings() {

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

		//transmissionFrequency
		if adapterSettings["transmissionFrequency"] != nil {
			log.Printf("[DEBUG] applyAdapterConfig - Setting transmissionFrequency to %s", adapterSettings["transmissionFrequency"].(string)+"\n")
			transmissionFrequency = adapterSettings["transmissionFrequency"].(string)
		} else {
			adapterSettings["transmissionFrequency"] = transmissionFrequency
		}
	}

	//transmissionDataRate
	if adapterSettings["transmissionDataRate"] != nil {
		log.Printf("[DEBUG] applyAdapterConfig - Setting transmissionDataRate to %s", adapterSettings["transmissionDataRate"].(string)+"\n")
		transmissionDataRate = adapterSettings["transmissionDataRate"].(string)
	} else {
		adapterSettings["transmissionDataRate"] = transmissionDataRate
	}
}

func setSerialPortName() {
	// 1. Determine if we are running on a Multitech Conduit
	// 2. Determine which port (ap1 or ap2) has a product ID of MTAC-XDOT

	productID := getProductID("")
	if strings.Contains(productID, conduitProductIDPrefix) {
		log.Printf("[INFO] setSerialPortName - Multitech Conduit detected: %s\n", productID)
		//We are running on a Multitech Conduit. Use ap1 and ap2 as port names
		xDotPort = "ap1"
		portProductID := getProductID(xDotPort)
		if strings.Contains(portProductID, xDotProductID) {
			log.Printf("[INFO] setSerialPortName - XDOT detected on ap1\n")
			serialPortName = "/dev/ttyAP1"
		} else {
			xDotPort = "ap2"
			portProductID := getProductID(xDotPort)
			if strings.Contains(portProductID, xDotProductID) {
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

func getProductID(portName string) string {
	port := ""
	if portName != "" {
		port += portName + "/"
	}
	port += "product-id"

	return executeMtsIoCommand(port)
}

func executeMtsIoCommand(mtsioObject string) string {
	cmd := exec.Command(mtsIoCmd, "show", mtsioObject)
	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		log.Printf("[ERROR] executeMtsIoCommand - ERROR executing mts-io-sysfs command: %s\n", err.Error())
		return ""
	}
	log.Printf("[DEBUG] executeMtsIoCommand - Command response received: %s\n", out.String())
	return out.String()
}

func resetXdot() string {
	cmd := exec.Command(xDotUtilCmd, "reset")
	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		log.Printf("[ERROR] resetXdot - ERROR executing xdot-util command: %s\n", err.Error())
		return ""
	}
	log.Printf("[DEBUG] resetXdot - Command response received: %s\n", out.String())
	return out.String()
}

func readFromSerialPort() {
	// 1. Read all data from serial port
	// 2. Publish data to platform as string
	var data string
	serialPortLock.Lock()
	buffer, err := serialPort.ReadSerialPort()
	for err == nil {
		data += buffer
		buffer, err = serialPort.ReadSerialPort()
	}
	serialPortLock.Unlock()

	if err != nil && !strings.Contains(err.Error(), "EOF") {
		log.Printf("[ERROR] readFromSerialPort - ERROR reading from serial port: %s\n", err.Error())
	} else {
		if data != "" {
			//If there are any slashes in the data, we need to escape them so duktape
			//doesn't throw a SyntaxError: unterminated string (line 1) error
			data = strings.Replace(data, `\`, `\\`, -1)

			log.Printf("[INFO] readFromSerialPort - Data read from serial port: %s\n", data)

			//Publish data to message broker
			err := publish(topicRoot+"/"+serialRead+"/response", data)
			if err != nil {
				log.Printf("[ERROR] readFromSerialPort - ERROR publishing to topic: %s\n", err.Error())
			}
		} else {
			log.Println("[DEBUG] readFromSerialPort - No data read from serial port, skipping publish.")
		}
	}
	return
}

func writeToSerialPort(payload string) {
	log.Printf("[INFO] writeToSerialPort - Writing to serial port: %s\n", payload)
	log.Println("[DEBUG] writeToSerialPort - About to lock serialPortLock")
	serialPortLock.Lock()
	err := serialPort.WriteSerialPort(string(payload))
	serialPortLock.Unlock()
	log.Println("[DEBUG] writeToSerialPort - Just unlocked serialPortLock")
	if err != nil {
		log.Printf("[ERROR] writeToSerialPort - ERROR writing to serial port: %s\n", err.Error())
	}
	return
}
