# xDot Adapter

The __xDot__ adapter provides the ability for the ClearBlade platform to communicate with a MultiTech __MultiConnect xDot__ device (https://www.multitech.com/models/94558032LF). The MultiConnect xDot is a LoRaWAN, low-power RF device. Interacting with the xDot is done through the use of AT commands (https://www.multitech.com/documents/publications/manuals/s000643.pdf).

As currently written, the __xDot__ adapter enables peer-to-peer (http://www.multitech.net/developer/software/mdot-software/peer-to-peer/) communications with other xDot devices. Peer-to-peer mode provides the ability to create a _mesh_ network of LoRa enabled devices. In additon, the xDot adapter enables _serial data mode_ on the xDot device in order to transmit and receive data in _peer-to-peer_ mode.

The adapter subscribes to MQTT topics which are used to interact with the xDot. The adapter publishes any data retrieved from the xDot to MQTT topics so that the ClearBlade Platform is able to retrieve and process xDot related data or write data to xDot devices.

# MQTT Topic Structure
The xDot adapter utilizes MQTT messaging to communicate with the ClearBlade Platform. The xDot adapter will subscribe to a specific topic in order to handle xDot device requests. Additionally, the xDot adapter will publish messages to MQTT topics in order to communicate the results of requests to client applications. The topic structures utilized by the xDot adapter are as follows:

  * Read xDot data request: {__TOPIC ROOT__}/receive/request
  * Read xDot data response: {__TOPIC ROOT__}/receive/response
  * Write xDot data request: {__TOPIC ROOT__}/send/request


## ClearBlade Platform Dependencies
The xDot adapter was constructed to provide the ability to communicate with a _System_ defined in a ClearBlade Platform instance. Therefore, the adapter requires a _System_ to have been created within a ClearBlade Platform instance.

Once a System has been created, artifacts must be defined within the ClearBlade Platform system to allow the adapters to function properly. At a minimum: 

  * A device needs to be created in the Auth --> Devices collection. The device will represent the adapter, or more importantly, the xDot device or MultiTech Conduit gateway on which the adapter is executing. The _name_ and _active key_ values specified in the Auth --> Devices collection will be used by the adapter to authenticate to the ClearBlade Platform or ClearBlade Edge. 
  * An adapter configuration data collection needs to be created in the ClearBlade Platform _system_ and populated with the data appropriate to the xDot adapter. The schema of the data collection should be as follows:


| Column Name      | Column Datatype |
| ---------------- | --------------- |
| adapter_name     | string          |
| topic_root       | string          |
| adapter_settings | string (json)   |

### adapter_settings
The adapter_settings column will need to contain a JSON object containing the following attributes:

##### networkAddress
* 4 bytes of hex data using a colon (:) to separate each byte from the next byte
* __Must be identical on all xDots in order for peer-to-peer mode to function__

##### networkDataKey
* 16 bytes of hex data using a colon (:) to separate each byte from the next byte
* __Must be identical on all xDots in order for peer-to-peer mode to function__

##### networkSessionKey
* 16 bytes of hex data using a colon (:) to separate each byte from the next byte
* __Must be identical on all xDots in order for peer-to-peer mode to function__

##### serialPortName
* The full unix path to the xDot serial device (ex. /dev/ttyAP1)

##### transmissionDataRate
* DR0-DR15 can be used
* See https://www.multitech.com/documents/publications/manuals/s000643.pdf for further information

##### transmissionFrequency
* The transmit frequency to use in peer-to-peer mode
* Use 915.5-919.7 MhZ for US 915 devices to avoid interference with LoRaWAN networks

#### adapter_settings_example
{  
  "networkAddress":"00:11:22:33",  
  "networkDataKey":"33:22:11:00:33:22:11:00:33:22:11:00:33:22:11:00",   
  "networkSessionKey":"00:11:22:33:00:11:22:33:00:11:22:33:00:11:22:33",  
  "serialPortName":"/dev/ttyAP1",  
  "transmissionDataRate":"DR8",  
  "transmissionFrequency":"915500000"  
}


## Usage

### Executing the adapter

`xDotAdapter -systemKey=<SYSTEM_KEY> -systemSecret=<SYSTEM_SECRET> -platformURL=<PLATFORM_URL> -messagingURL=<MESSAGING_URL> -deviceName=<DEVICE_NAME> -password=<DEVICE_ACTIVE_KEY> -adapterConfigCollectionID=<COLLECTION_ID> -logLevel=<LOG_LEVEL>`

   __*Where*__ 

   __systemKey__
  * REQUIRED
  * The system key of the ClearBLade Platform __System__ the adapter will connect to

   __systemSecret__
  * REQUIRED
  * The system secret of the ClearBLade Platform __System__ the adapter will connect to
   
   __deviceName__
  * The device name the adapter will use to authenticate to the ClearBlade Platform
  * Requires the device to have been defined in the _Auth - Devices_ collection within the ClearBlade Platform __System__
  * OPTIONAL
  * Defaults to __xDotSerialAdapter__
   
   __password__
  * REQUIRED
  * The active key the adapter will use to authenticate to the platform
  * Requires the device to have been defined in the _Auth - Devices_ collection within the ClearBlade Platform __System__
   
   __platformUrl__
  * The url of the ClearBlade Platform instance the adapter will connect to
  * OPTIONAL
  * Defaults to __http://localhost:9000__

   __messagingUrl__
  * The MQTT url of the ClearBlade Platform instance the adapter will connect to
  * OPTIONAL
  * Defaults to __localhost:1883__

   __adapterConfigCollectionID__
  * REQUIRED 
  * The collection ID of the data collection used to house adapter configuration data

   __logLevel__
  * The level of runtime logging the adapter should provide.
  * Available log levels:
    * fatal
    * error
    * warn
    * info
    * debug
  * OPTIONAL
  * Defaults to __info__


## Setup
---
The xdot adapters are dependent upon the ClearBlade Go SDK and its dependent libraries being installed. The xDot adapter was written in Go and therefore requires Go to be installed (https://golang.org/doc/install).


### Adapter compilation
In order to compile the adapter for execution within mLinux, the following steps need to be performed:

 1. Retrieve the adapter source code  
    * ```git clone git@github.com:ClearBlade/xDot-Adapter.git```
 2. Navigate to the xdotadapter directory  
    * ```cd xdotadapter```
 4. Compile the adapter
    * ```GOARCH=arm GOARM=5 GOOS=linux go build```



