package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	"xDotAdapter/xDotSerial"

	cb "github.com/clearblade/Go-SDK"
	mqttTypes "github.com/clearblade/mqtt_parsing"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/hashicorp/logutils"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

const (
	platURL         = "http://localhost:9000"
	messURL         = "localhost:1883"
	msgSubscribeQos = 0
	msgPublishQos   = 0
	serialRead      = "receive"
	serialWrite     = "send"
)

var (
	platformURL         string //Defaults to http://localhost:9000
	messagingURL        string //Defaults to localhost:1883
	sysKey              string
	sysSec              string
	deviceName          string //Defaults to xDotSerialAdapter
	activeKey           string
	logLevel            string //Defaults to info
	adapterConfigCollID string

	serialPortName        = "/dev/ttyAP1"
	networkAddress        = "00:11:22:33"
	networkSessionKey     = "00:11:22:33:00:11:22:33:00:11:22:33:00:11:22:33"
	networkDataKey        = "33:22:11:00:33:22:11:00:33:22:11:00:33:22:11:00"
	transmissionDataRate  = "DR8"
	transmissionFrequency = "915500000"
	topicRoot             = "wayside/lora"

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

	//Initialize the logging mechanism
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"},
		MinLevel: logutils.LogLevel(strings.ToUpper(logLevel)),
		Writer: &lumberjack.Logger{
			Filename:   "/var/log/xDotAdapter.log",
			MaxSize:    10, // megabytes
			MaxBackups: 5,
			MaxAge:     28, //days
		},
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
	var err error
	if err = initCbClient(cbBroker); err != nil {
		log.Println(err.Error())
		log.Println("Unable to initialize CB broker client. Exiting.")
		return
	}

	defer close(endWorkersChannel)
	endWorkersChannel = make(chan string)
	done := make(chan bool)

	<-done
}

// ClearBlade Client init helper
func initCbClient(platformBroker cbPlatformBroker) error {
	log.Println("[DEBUG] initCbClient - Initializing the ClearBlade client")

	cbBroker.client = cb.NewDeviceClientWithAddrs(*(platformBroker.platformURL), *(platformBroker.messagingURL), *(platformBroker.systemKey), *(platformBroker.systemSecret), *(platformBroker.username), *(platformBroker.password))

	for err := cbBroker.client.Authenticate(); err != nil; {
		log.Printf("[ERROR] initCbClient - Error authenticating %s: %s\n", platformBroker.name, err.Error())
		log.Println("[ERROR] initCbClient - Will retry in 1 minute...")

		// sleep 1 minute
		time.Sleep(time.Duration(time.Minute * 1))
		err = cbBroker.client.Authenticate()
	}

	//Retrieve adapter configuration data
	log.Println("[INFO] main - Retrieving adapter configuration...")
	query, adapter_settings := getAdapterConfig()

	serialPort = xDotSerial.CreateXdotSerialPort(serialPortName, 115200, time.Millisecond*2500)

	log.Println("[DEBUG] main - Opening serial port")
	if err := serialPort.OpenSerialPort(); err != nil {
		log.Panic("[FATAL] main - Error opening serial port: " + err.Error())
	}

	//Turn off serial data mode in case it is currently on
	if err := serialPort.StopSerialDataMode(); err != nil {
		log.Println("[WARN] main - Error stopping serial data mode: " + err.Error())
	}

	//Initialize xDot network settings and data rate
	log.Println("[DEBUG] main - Configuring xDot")
	configureXDot()

	log.Println("[DEBUG] main - Updating device ID")
	retrieveAndSaveDeviceId(&query, adapter_settings)

	log.Println("[DEBUG] initCbClient - Initializing MQTT")
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

	//Start subscribe worker
	go subscribeWorker()
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
	log.Println("[DEBUG] configureXDot - Setting network join mode...")
	if valueChanged, err := serialPort.SetNetworkJoinMode(xDotSerial.PeerToPeerMode); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set the device class to class C
	log.Println("[DEBUG] configureXDot - Setting device class...")
	if valueChanged, err := serialPort.SetDeviceClass(xDotSerial.DeviceClassC); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set network address
	log.Println("[DEBUG] configureXDot - Setting network address...")
	if valueChanged, err := serialPort.SetNetworkAddress(networkAddress); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set network session key
	log.Println("[DEBUG] configureXDot - Setting network session key...")
	if valueChanged, err := serialPort.SetNetworkSessionKey(networkSessionKey); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set data session key
	log.Println("[DEBUG] configureXDot - Setting data session key...")
	if valueChanged, err := serialPort.SetDataSessionKey(networkDataKey); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set transmission data rate
	log.Println("[DEBUG] configureXDot - Setting transmission data rate...")
	if valueChanged, err := serialPort.SetDataRate(transmissionDataRate); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	//Set transmission frequency
	log.Println("[DEBUG] configureXDot - Setting transmission frequency...")
	if valueChanged, err := serialPort.SetFrequency(transmissionFrequency); err != nil {
		panic(err.Error())
	} else {
		if valueChanged == true {
			serialConfigChanged = true
		}
	}

	if serialConfigChanged == true {
		log.Println("[DEBUG] configureXDot - xDot configuration changed, saving new values...")
		//Save the xDot configuration
		if err := serialPort.SaveConfiguration(); err != nil {
			log.Println("[WARN] configureXDot - Error saving xDot configuration: " + err.Error())
		} else {
			//Reset the xDot CPU
			log.Println("[DEBUG] configureXDot - Resetting xDot CPU...")
			if err := serialPort.ResetXDotCPU(); err != nil {
				log.Panic("[FATAL] configureXDot - Error resetting xDot CPU: " + err.Error())
			}
		}
	}
}

func subscribeWorker() {
	log.Println("[DEBUG] subscribeWorker - Starting subscribeWorker")

	isReading := false
	isWriting := false

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

					log.Println("[DEBUG] subscribeWorker - Read request received")
					// If read request...
					//
					// 1. Read all data from serial port
					// 2. Publish data to platform as string

					var data string

					for isWriting {
						log.Println("[INFO] subscribeWorker - Currently writing to serial port. Waiting 1 second...")
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
						log.Printf("[ERROR] subscribeWorker - ERROR reading from serial port: %s\n", err.Error())
					} else {
						log.Printf("[DEBUG] subscribeWorker - Data read from serial port: %#v\n", data)

						if data != "" {
							//Publish data to message broker
							err := publish(topicRoot+"/"+serialRead+"/response", data)
							if err != nil {
								log.Printf("[ERROR] subscribeWorker - ERROR publishing to topic: %s\n", err.Error())
							}
						} else {
							log.Println("[DEBUG] subscribeWorker - No data read from serial port, skipping publish.")
						}
					}
				} else if strings.HasSuffix(message.Topic.Whole, serialWrite+"/request") {
					log.Printf("[DEBUG] subscribeWorker - Write request received: %#v\n", message.Payload)
					// If write request...
					// Write string to serial port
					for isReading {
						log.Println("[INFO] subscribeWorker - Currently reading from serial port. Waiting 1 second...")
						time.Sleep(1 * time.Second)
					}

					log.Printf("[DEBUG] subscribeWorker - Writing to serial port: %#v\n", message.Payload)
					isWriting = true
					err := serialPort.WriteSerialPort(string(message.Payload))
					isWriting = false
					if err != nil {
						log.Printf("[ERROR] subscribeWorker - ERROR writing to serial port: %s\n", err.Error())
					}
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

func getAdapterConfig() (cb.Query, map[string]interface{}) {
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

	return *query, settingsJson
}

func applyAdapterSettings(adapterSettings map[string]interface{}) {
	//serialPortName
	if adapterSettings["serialPortName"] != nil {
		log.Printf("[DEBUG] applyAdapterConfig - Setting serialPortName to %s", adapterSettings["serialPortName"].(string)+"\n")
		serialPortName = adapterSettings["serialPortName"].(string)
	} else {
		adapterSettings["serialPortName"] = serialPortName
	}

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

func retrieveAndSaveDeviceId(query *cb.Query, adapterSettings map[string]interface{}) {
	deviceID, err := serialPort.GetDeviceID()
	if err != nil {
		log.Printf("[ERROR] retrieveAndSaveDeviceId - ERROR retrieving device ID: %s\n", err.Error())
	}
	log.Printf("[DEBUG] retrieveAndSaveDeviceId - Retrieved device ID: %s\n", deviceID)

	//Store device ID in adapter_settings within adapter_config collection
	deviceID = strings.Replace(deviceID, "-", ":", -1)

	log.Printf("[DEBUG] adapterSettings: %#v\n", deviceID)
	if adapterSettings["eui"] == nil || adapterSettings["eui"] != deviceID {
		adapterSettings["eui"] = deviceID

		if settingsBytes, err := json.Marshal(adapterSettings); err != nil {
			log.Printf("[DEBUG] retrieveAndSaveDeviceId - Error marshalling json: %s\n", err.Error())
		} else {
			changes := make(map[string]interface{})
			changes["adapter_settings"] = string(settingsBytes)

			log.Printf("[DEBUG] CHANGES = : %#v\n", changes)

			err = cbBroker.client.UpdateData(adapterConfigCollID, query, changes)
			if err != nil {
				log.Printf("[ERROR] retrieveAndSaveDeviceId - ERROR updating adapter_settings column: %s", err.Error())
			}
		}
	}
}
