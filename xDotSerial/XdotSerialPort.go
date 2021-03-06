package xDotSerial

import (
	"errors"
	"io"
	"log"
	"strings"
	"time"

	"github.com/tarm/serial"
)

//XdotSerialPort - Struct that represents the serial port used to interface with a Xdot
type XdotSerialPort struct {
	serialPort *serial.Port

	//The OS name used to identify the port (/dev/ttyap1, COM1, etc)
	portName string

	//The baud rate of the serial port, default is 115200
	baudRate int

	//
	timeout time.Duration
}

func CreateXdotSerialPort(osName string, baud int, timeout time.Duration) *XdotSerialPort {

	var thePort = XdotSerialPort{portName: osName, baudRate: baud, timeout: timeout}
	return &thePort
}

func (xDot *XdotSerialPort) OpenSerialPort() error {
	log.Println("[DEBUG] OpenSerialPort - Opening serial port")

	serialConfig := &serial.Config{Name: xDot.portName, Baud: xDot.baudRate, ReadTimeout: xDot.timeout}

	var err error

	xDot.serialPort, err = serial.OpenPort(serialConfig)
	if err != nil {
		log.Println("[ERROR] OpenSerialPort - Error opening serial port: " + err.Error())
		return err
	}

	log.Println("[INFO] OpenSerialPort - Serial port open")

	return nil
}

func (xDot *XdotSerialPort) CloseSerialPort() error {
	var err error
	log.Println("[DEBUG] CloseSerialPort - Closing serial port")
	err = xDot.serialPort.Close()
	if err != nil {
		log.Println("[ERROR] CloseSerialPort - Error closing serial port: " + err.Error())
		return err
	}
	log.Println("[INFO] CloseSerialPort - Serial port closed")
	return nil
}

func (xDot *XdotSerialPort) SendATCommand(cmd string) (string, error) {
	atCmd := cmd + "\r"

	//write to the serial port
	log.Printf("[DEBUG] SendATCommand - Writing AT Command to serial port: %#v\n", atCmd)

	n, err := xDot.serialPort.Write([]byte(atCmd))
	if err != nil {
		log.Printf("[ERROR] SendATCommand - ERROR writing AT command to serial port: %s\n", err.Error())
		return "", err
	}
	if n == -1 {
		log.Printf("[ERROR] SendATCommand - Bad return code received when executing AT command: %s\n", atCmd)
		return "", errors.New("-1 return code received when executing AT command")
	} else {
		log.Printf("[DEBUG] SendATCommand - Number of bytes written: %d\n", n)
	}

	resp, err := xDot.readCommandResponse()
	if err != nil {
		return "", err
	}

	log.Println("[DEBUG] SendATCommand - Returning AT command response: " + resp)
	return resp, nil
}

func (xDot *XdotSerialPort) readCommandResponse() (string, error) {
	sleepTime := 2500 * time.Millisecond

	// Every AT command will return either "OK\r\n", "ERROR\r\n", or "CONNECT\r\n" (In the
	// case of enabling serial data mode)
	// Read from the response from the serial port until either "OK\r\n" or "ERROR\r\n" are returned

	log.Println("[DEBUG] readCommandResponse - Reading response from serial port")

	resp := ""
	numTries := 1

	buff, err := xDot.ReadSerialPort()
	if err != nil && err != io.EOF {
		log.Println("[FATAL] readCommandResponse - Error Reading serial data response from serial port: " + err.Error())
		panic(err.Error())
	}
	resp += buff
	log.Printf("[DEBUG] readCommandResponse - Buffer read: %s\n", buff)

	for !strings.Contains(resp, AtCmdSuccessText) &&
		!strings.Contains(resp, AtCmdErrorText) &&
		!strings.Contains(resp, AtCmdConnectText) && numTries <= 5 {

		log.Println("[DEBUG] readCommandResponse - Continuing to read response from serial port...")

		time.Sleep(sleepTime)

		buff, err = xDot.ReadSerialPort()
		if err != nil && err == io.EOF {
			numTries++
			continue
		} else if err != nil {
			log.Println("[FATAL] readCommandResponse - Error Reading serial data response from serial port: " + err.Error())
			panic(err.Error())
		}
		log.Printf("[DEBUG] readCommandResponse - Buffer read: %s\n", buff)
		resp += buff
		numTries++
	}

	if numTries > 5 {
		log.Printf("[ERROR] readCommandResponse - Unable to read AT command response after %d milliseconds\n", sleepTime*5)
		return "", errors.New("Unable to read response after " + string(sleepTime*5) + " milliseconds")
	}

	log.Println("[DEBUG] readCommandResponse - Finished retrieving AT command response")
	if strings.Contains(resp, AtCmdErrorText) {
		log.Println("[DEBUG] readCommandResponse - Error received executing AT command: " + resp)
		return "", errors.New(resp)
	}

	log.Println("[DEBUG] readCommandResponse - Returning AT command response: " + resp)
	return resp, nil
}

func (xDot *XdotSerialPort) GetDeviceID() (string, error) {
	log.Println("[DEBUG] GetDeviceID - Retrieving device ID")

	if response, err := xDot.SendATCommand(DeviceIDCmd); err != nil {
		log.Println("[ERROR] GetDeviceID - Error retrieving device ID: " + err.Error())
		return "", err
	} else {
		return xDot.ExtractResponseData(DeviceIDCmd, response), nil
	}
}

func (xDot *XdotSerialPort) GetDeviceClass() (string, error) {
	log.Println("[DEBUG] GetDeviceClass - Retrieving device class")

	if response, err := xDot.SendATCommand(DeviceClassCmd); err != nil {
		log.Println("[ERROR] GetDeviceClass - Error retrieving device class: " + err.Error())
		return "", err
	} else {
		return xDot.ExtractResponseData(DeviceClassCmd, response), nil
	}
}

func (xDot *XdotSerialPort) SetDeviceClass(devClass string) (bool, error) {
	log.Println("[DEBUG] SetDeviceClass - Begin setting device class to " + devClass)

	//Retrieve the current value. Don't update it if we don't have to
	currentValue, err := xDot.GetDeviceClass()

	log.Println("[DEBUG] SetDeviceClass - Current value = " + currentValue)
	log.Println("[DEBUG] SetDeviceClass - Value to set = " + devClass)

	if err != nil || !strings.Contains(strings.ToLower(currentValue), strings.ToLower(devClass)) {
		if err != nil {
			log.Println("[ERROR] SetDeviceClass - Error retrieving device class: " + err.Error())
			return false, err
		} else {
			if _, err := xDot.SendATCommand(DeviceClassCmd + "=" + devClass); err != nil {
				log.Println("[ERROR] SetDeviceClass - Error setting device class: " + err.Error())
				return false, err
			}
			log.Println("[INFO] SetDeviceClass - Device class set to " + devClass)
			return true, nil
		}
	}
	log.Println("[INFO] SetDeviceClass - Device class (unchanged) = " + devClass)
	return false, nil
}

func (xDot *XdotSerialPort) GetNetworkJoinMode() (string, error) {
	log.Println("[DEBUG] GetNetworkJoinMode - Retrieving network join mode")
	if response, err := xDot.SendATCommand(NetworkJoinModeCmd); err != nil {
		log.Println("[ERROR] GetNetworkJoinMode - Error retrieving network join mode: " + err.Error())
		return "", err
	} else {
		return xDot.ExtractResponseData(NetworkJoinModeCmd, response), nil
	}
}

func (xDot *XdotSerialPort) SetNetworkJoinMode(mode string) (bool, error) {
	log.Println("[DEBUG] SetNetworkJoinMode - Begin setting network join mode to " + mode)

	//Retrieve the current value. Don't update it if we don't have to
	currentValue, err := xDot.GetNetworkJoinMode()

	log.Println("[DEBUG] SetNetworkJoinMode - Current value = " + currentValue)
	log.Println("[DEBUG] SetNetworkJoinMode - Value to set = " + mode)

	if err != nil || !strings.Contains(currentValue, mode) {
		if err != nil {
			log.Println("[ERROR] SetNetworkJoinMode - Error retrieving network join mode: " + err.Error())
			return false, err
		} else {
			if _, err := xDot.SendATCommand(NetworkJoinModeCmd + "=" + mode); err != nil {
				log.Println("[ERROR] SetNetworkJoinMode - Error setting network join mode: " + err.Error())
				return false, err
			}
			log.Println("[INFO] SetNetworkJoinMode - Network join set to " + mode)
			return true, nil
		}
	}
	log.Println("[INFO] SetNetworkJoinMode - Network join (unchanged) = " + mode)
	return false, nil
}

func (xDot *XdotSerialPort) GetNetworkAddress() (string, error) {
	log.Println("[DEBUG] getNetworGetNetworkAddresskAddress - Retrieving network address")
	if response, err := xDot.SendATCommand(NetworkAddrCmd); err != nil {
		log.Println("[ERROR] GetNetworkAddress - Error retrieving network address: " + err.Error())
		return "", err
	} else {
		return xDot.ExtractResponseData(NetworkAddrCmd, response), nil
	}
}

func (xDot *XdotSerialPort) SetNetworkAddress(address string) (bool, error) {
	log.Println("[DEBUG] SetNetworkAddress - Setting network address")

	//Retrieve the current value. Don't update it if we don't have to
	currentValue, err := xDot.GetNetworkAddress()

	log.Println("[DEBUG] SetNetworkAddress - Current value = " + currentValue)
	log.Println("[DEBUG] SetNetworkAddress - Value to set = " + address)

	if err != nil || !strings.Contains(strings.ToLower(currentValue), strings.ToLower(address)) {
		if err != nil {
			log.Println("[DEBUG] SetNetworkAddress - Error retrieving network address: " + err.Error())
			return false, err
		} else {
			if _, err := xDot.SendATCommand(NetworkAddrCmd + "=" + address); err != nil {
				log.Println("[ERROR] SetNetworkAddress - Error setting network address: " + err.Error())
				return false, err
			}
			log.Println("[INFO] SetNetworkAddress - Network address set to " + address)
			return true, nil
		}
	}

	log.Println("[INFO] SetNetworkAddress - Network address (unchanged) = " + address)
	return false, nil
}

func (xDot *XdotSerialPort) GetNetworkSessionKey() (string, error) {
	log.Println("[DEBUG] GetNetworkSessionKey - Retrieving network session key")
	if response, err := xDot.SendATCommand(NetworkSessionKeyCmd); err != nil {
		log.Println("[ERROR] GetNetworkSessionKey - Error setting network session key: " + err.Error())
		return "", err
	} else {
		return xDot.ExtractResponseData(NetworkSessionKeyCmd, response), nil
	}
}

func (xDot *XdotSerialPort) SetNetworkSessionKey(sessionKey string) (bool, error) {
	log.Println("[DEBUG] SetNetworkSessionKey - Setting network session key")
	//Retrieve the current value. Don't update it if we don't have to
	currentValue, err := xDot.GetNetworkSessionKey()

	//Replace periods with colons in the returned value
	currentValue = strings.Replace(currentValue, ".", ":", -1)

	log.Println("[DEBUG] SetNetworkSessionKey - Current value = " + currentValue)
	log.Println("[DEBUG] SetNetworkSessionKey - Value to set = " + sessionKey)

	if err != nil || !strings.Contains(strings.ToLower(currentValue), strings.ToLower(sessionKey)) {
		if err != nil {
			log.Println("[ERROR] SetNetworkSessionKey - Error retrieving network session key: " + err.Error())
			return false, err
		} else {
			if _, err := xDot.SendATCommand(NetworkSessionKeyCmd + "=" + sessionKey); err != nil {
				log.Println("[ERROR] SetNetworkSessionKey - Error setting network session key: " + err.Error())
				return false, err
			}
			log.Println("[INFO] SetNetworkSessionKey - Network session key set to " + sessionKey)
			return true, nil
		}
	}

	log.Println("[INFO] SetNetworkSessionKey - Network session key (unchanged) = " + sessionKey)
	return false, nil
}

func (xDot *XdotSerialPort) GetDataSessionKey() (string, error) {
	log.Println("[DEBUG] GetDataSessionKey - Retrieving data session key")
	if response, err := xDot.SendATCommand(NetworkDataKeyCmd); err != nil {
		log.Println("[ERROR] GetDataSessionKey - Error retrieving data session key" + err.Error())
		return "", err
	} else {
		return xDot.ExtractResponseData(NetworkDataKeyCmd, response), nil
	}
}

func (xDot *XdotSerialPort) SetDataSessionKey(dataKey string) (bool, error) {
	log.Println("[DEBUG] SetDataSessionKey - Setting data session key")
	//Retrieve the current value. Don't update it if we don't have to
	currentValue, err := xDot.GetDataSessionKey()

	//Replace periods with colons in the returned value
	currentValue = strings.Replace(currentValue, ".", ":", -1)

	log.Println("[DEBUG] SetDataSessionKey - Current value = " + currentValue)
	log.Println("[DEBUG] SetDataSessionKey - Value to set = " + dataKey)

	if err != nil || !strings.Contains(strings.ToLower(currentValue), strings.ToLower(dataKey)) {
		if err != nil {
			log.Println("[ERROR] SetDataSessionKey - Error retrieving data session key")
			return false, err
		} else {
			if _, err := xDot.SendATCommand(NetworkDataKeyCmd + "=" + dataKey); err != nil {
				log.Println("[ERROR] SetDataSessionKey - Error setting data session key")
				return false, err
			}
			log.Println("[INFO] SetDataSessionKey - Data session key set to " + dataKey)
			return true, nil
		}
	}

	log.Println("[INFO] SetDataSessionKey - Data session (unchanged) = " + dataKey)
	return false, nil
}

func (xDot *XdotSerialPort) GetDataRate() (string, error) {
	log.Println("[DEBUG] GetDataRate - Retrieving data transmission rate")
	if response, err := xDot.SendATCommand(TransmissionDataRateCmd); err != nil {
		log.Println("[ERROR] GetDataRate - Error retrieving data transmission rate")
		return "", err
	} else {
		return xDot.ExtractResponseData(TransmissionDataRateCmd, response), nil
	}
}

func (xDot *XdotSerialPort) SetDataRate(dataRate string) (bool, error) {
	log.Println("[DEBUG] SetDataRate - Setting data transmission rate")
	//Retrieve the current value. Don't update it if we don't have to
	currentValue, err := xDot.GetDataRate()

	log.Println("[DEBUG] SetDataRate - Current value = " + currentValue)
	log.Println("[DEBUG] SetDataRate - Value to set = " + dataRate)

	if err != nil || !strings.Contains(currentValue, dataRate) {
		if err != nil {
			log.Println("[DEBUG] SetDataRate - Error retrieving data transmission rate")
			return false, err
		} else {
			if _, err := xDot.SendATCommand(TransmissionDataRateCmd + "=" + dataRate); err != nil {
				log.Println("[DEBUG] SetDataRate - Error setting data transmission rate")
				return false, err
			}
			log.Println("[INFO] SetDataRate - Data transmission rate set to " + dataRate)
			return true, nil
		}
	}

	log.Println("[INFO] SetDataRate - Data transmission (unchanged) = " + dataRate)
	return false, nil
}

func (xDot *XdotSerialPort) GetAntennaGain() (string, error) {
	log.Println("[DEBUG] GetAntennaGain - Retrieving antenna gain")

	response, err := xDot.SendATCommand(AntennaGainCmd)
	if err != nil {
		log.Println("[ERROR] GetAntennaGain - Error retrieving antenna gain")
		return "", err
	}
	return xDot.ExtractResponseData(AntennaGainCmd, response), nil
}

func (xDot *XdotSerialPort) SetAntennaGain(antennaGain string) (bool, error) {
	log.Println("[DEBUG] SetAntennaGain - Setting antenna gain")
	//Retrieve the current value. Don't update it if we don't have to
	currentValue, err := xDot.GetAntennaGain()

	log.Println("[DEBUG] SetAntennaGain - Current value = " + currentValue)
	log.Println("[DEBUG] SetAntennaGain - Value to set = " + antennaGain)

	if err != nil || !strings.Contains(currentValue, antennaGain) {
		if err != nil {
			log.Println("[DEBUG] SetAntennaGain - Error retrieving antenna gain")
			return false, err
		}
		if _, err := xDot.SendATCommand(AntennaGainCmd + "=" + antennaGain); err != nil {
			log.Println("[DEBUG] SetAntennaGain - Error setting antenna gain")
			return false, err
		}
		log.Println("[INFO] SetAntennaGain - Antenna gain set to " + antennaGain)
		return true, nil
	}

	log.Println("[INFO] SetAntennaGain - Antenna gain (unchanged) = " + antennaGain)
	return false, nil
}

func (xDot *XdotSerialPort) GetTransmitPower() (string, error) {
	log.Println("[DEBUG] GetTransmitPower - Retrieving transmit power")
	response, err := xDot.SendATCommand(TransmitPowerCmd)
	if err != nil {
		log.Println("[ERROR] GetTransmitPower - Error retrieving transmit power")
		return "", err
	}
	return xDot.ExtractResponseData(TransmitPowerCmd, response), nil
}

func (xDot *XdotSerialPort) SetTransmitPower(xmitPower string) (bool, error) {
	log.Println("[DEBUG] SetTransmitPower - Setting transmit power")
	//Retrieve the current value. Don't update it if we don't have to
	currentValue, err := xDot.GetTransmitPower()

	log.Println("[DEBUG] SetTransmitPower - Current value = " + currentValue)
	log.Println("[DEBUG] SetTransmitPower - Value to set = " + xmitPower)

	if err != nil || !strings.Contains(currentValue, xmitPower) {
		if err != nil {
			log.Println("[DEBUG] SetTransmitPower - Error retrieving transmit power")
			return false, err
		}
		if _, err := xDot.SendATCommand(TransmitPowerCmd + "=" + xmitPower); err != nil {
			log.Println("[DEBUG] SetTransmitPower - Error setting transmit power")
			return false, err
		}
		log.Println("[INFO] SetTransmitPower - Transmit power set to " + xmitPower)
		return true, nil
	}

	log.Println("[INFO] SetTransmitPower - Transmit power (unchanged) = " + xmitPower)
	return false, nil
}

func (xDot *XdotSerialPort) GetFrequency() (string, error) {
	log.Println("[DEBUG] GetFrequency - Retrieving frequency")
	if response, err := xDot.SendATCommand(TransmissionFrequencyCmd); err != nil {
		log.Println("[ERROR] GetFrequency - Error retrieving frequency: " + err.Error())
		return "", err
	} else {
		return xDot.ExtractResponseData(TransmissionFrequencyCmd, response), nil
	}
}

func (xDot *XdotSerialPort) SetFrequency(freq string) (bool, error) {
	log.Println("[DEBUG] SetFrequency - Setting data transmission frequency")
	//Retrieve the current value. Don't update it if we don't have to
	currentValue, err := xDot.GetFrequency()

	log.Println("[DEBUG] SetFrequency - Current value = " + currentValue)
	log.Println("[DEBUG] SetFrequency - Value to set = " + freq)

	if err != nil || !strings.Contains(currentValue, freq) {
		if err != nil {
			log.Println("[ERROR] SetFrequency - Error retrieving data transmission frequency: " + err.Error())
			return false, err
		} else {
			if _, err := xDot.SendATCommand(TransmissionFrequencyCmd + "=" + freq); err != nil {
				log.Println("[ERROR] SetFrequency - Error setting data transmission frequency: " + err.Error())
				return false, err
			}
			log.Println("[INFO] SetFrequency - Data transmission frequency set to " + freq)
			return true, nil
		}
	}
	log.Println("[INFO] SetFrequency - Data transmission frequency (unchanged) = " + freq)
	return false, nil
}

func (xDot *XdotSerialPort) GetPublicNetworkMode() (string, error) {
	log.Println("[DEBUG] GetPublicNetworkMode - Retrieving public network mode")
	if response, err := xDot.SendATCommand(PublicNetworkModeCmd); err != nil {
		log.Println("[ERROR] GetPublicNetworkMode - Error retrieving public network mode: " + err.Error())
		return "", err
	} else {
		return xDot.ExtractResponseData(PublicNetworkModeCmd, response), nil
	}
}

func (xDot *XdotSerialPort) SetPublicNetworkMode(mode string) (bool, error) {
	log.Println("[DEBUG] SetPublicNetworkMode - Setting Public Network Mode")
	currentValue, err := xDot.GetPublicNetworkMode()
	if err != nil {
		log.Println("[ERROR] SetPublicNetworkMode - Error retrieving: " + err.Error())
		return false, err
	}
	log.Println("[DEBUG] SetPublicNetworkMode - Current value = " + currentValue)
	log.Println("[DEBUG] SetPublicNetworkMode - Value to set = " + mode)
	if currentValue == mode {
		log.Println("[INFO] SetPublicNetworkMode - Unchanged = " + mode)
		return false, nil
	}
	if _, err := xDot.SendATCommand(NetworkIdCmd + "=0," + mode); err != nil {
		log.Println("[ERROR] SetPublicNetworkMode - Error setting: " + err.Error())
		return false, err
	}
	log.Println("[INFO] SetPublicNetworkMode - Set to " + mode)
	return true, nil
}

func (xDot *XdotSerialPort) GetNetworkID() (string, error) {
	log.Println("[DEBUG] GetNetworkID - Retrieving Network ID")
	if response, err := xDot.SendATCommand(NetworkIdCmd); err != nil {
		log.Println("[ERROR] GetNetworkID - Error retrieving Network ID: " + err.Error())
		return "", err
	} else {
		return xDot.ExtractResponseData(NetworkIdCmd, response), nil
	}
}

func (xDot *XdotSerialPort) SetNetworkID(id string) (bool, error) {
	log.Println("[DEBUG] SetNetworkID - Setting Network ID")
	currentValue, err := xDot.GetNetworkID()
	if err != nil {
		log.Println("[ERROR] SetNetworkID - Error retrieving: " + err.Error())
		return false, err
	}
	log.Println("[DEBUG] SetNetworkID - Current value = " + currentValue)
	log.Println("[DEBUG] SetNetworkID - Value to set = " + id)
	if currentValue == id {
		log.Println("[INFO] SetNetworkID - Unchanged = " + id)
		return false, nil
	}
	if _, err := xDot.SendATCommand(NetworkIdCmd + "=0," + id); err != nil {
		log.Println("[ERROR] SetNetworkID - Error setting: " + err.Error())
		return false, err
	}
	log.Println("[INFO] SetNetworkID - Set to " + id)
	return true, nil
}

func (xDot *XdotSerialPort) GetNetworkKey() (string, error) {
	log.Println("[DEBUG] GetNetworkKey - Retrieving Network Key")
	if response, err := xDot.SendATCommand(NetworkKeyCmd); err != nil {
		log.Println("[ERROR] GetNetworkKey - Error retrieving: " + err.Error())
		return "", err
	} else {
		return xDot.ExtractResponseData(NetworkKeyCmd, response), nil
	}
}

func (xDot *XdotSerialPort) SetNetworkKey(key string) (bool, error) {
	log.Println("[DEBUG] SetNetworkKey - Setting Network Key")
	currentValue, err := xDot.GetNetworkKey()
	if err != nil {
		log.Println("[ERROR] SetNetworkKey - Error retrieving: " + err.Error())
		return false, err
	}
	log.Println("[DEBUG] SetNetworkKey - Current value = " + currentValue)
	log.Println("[DEBUG] SetNetworkKey - Value to set = " + key)
	if currentValue == key {
		log.Println("[INFO] SetNetworkKey - Unchanged = " + key)
		return false, nil
	}
	if _, err := xDot.SendATCommand(NetworkKeyCmd + "=0," + key); err != nil {
		log.Println("[ERROR] SetNetworkKey - Error setting: " + err.Error())
		return false, err
	}
	log.Println("[INFO] SetNetworkKey - Set to " + key)
	return true, nil
}

func (xDot *XdotSerialPort) GetFrequencySubBand() (string, error) {
	log.Println("[DEBUG] GetFrequencySubBand - Retrieving Frequency Sub-Band")
	if response, err := xDot.SendATCommand(FrequencySubBandCmd); err != nil {
		log.Println("[ERROR] GetFrequencySubBand - Error retrieving: " + err.Error())
		return "", err
	} else {
		return xDot.ExtractResponseData(FrequencySubBandCmd, response), nil
	}
}

func (xDot *XdotSerialPort) SetFrequencySubBand(subBand string) (bool, error) {
	log.Println("[DEBUG] SetFrequencySubBand - Setting Frequency Sub-Band")
	currentValue, err := xDot.GetFrequencySubBand()
	if err != nil {
		log.Println("[ERROR] SetFrequencySubBand - Error retrieving: " + err.Error())
		return false, err
	}
	log.Println("[DEBUG] SetFrequencySubBand - Current value = " + currentValue)
	log.Println("[DEBUG] SetFrequencySubBand - Value to set = " + subBand)
	if currentValue == subBand {
		log.Println("[INFO] SetFrequencySubBand - Unchanged = " + subBand)
		return false, nil
	}
	if _, err := xDot.SendATCommand(FrequencySubBandCmd + "=" + subBand); err != nil {
		log.Println("[ERROR] SetFrequencySubBand - Error setting: " + err.Error())
		return false, err
	}
	log.Println("[INFO] SetFrequencySubBand - Set to " + subBand)
	return true, nil
}

func (xDot *XdotSerialPort) JoinNetwork() error {
	log.Println("[DEBUG] JoinNetwork - Joining Network")
	if _, err := xDot.SendATCommand(NetworkJoinCmd); err != nil {
		log.Println("[ERROR] JoinNetwork - Failed to join network: " + err.Error())
		return err
	}
	return nil
}

func (xDot *XdotSerialPort) SendData(data string) error {
	log.Println("[DEBUG] SendData - Sending Data")
	command := SendDataCmd
	if data != "" {
		command += data
	}
	if _, err := xDot.SendATCommand(command); err != nil {
		log.Println("[ERROR] SendData - failed to send data: " + err.Error())
		return err
	}
	return nil
}

func (xDot *XdotSerialPort) SaveConfiguration() error {
	log.Println("[INFO] SaveConfiguration - Saving xDot configuration")
	if _, err := xDot.SendATCommand(SaveConfigurationCmd); err != nil {
		log.Println("[ERROR] SaveConfiguration - Error saving xDot configuration: " + err.Error())
		return err
	}
	return nil
}

func (xDot *XdotSerialPort) ResetXDotCPU() error {
	log.Println("[DEBUG] ResetXDotCPU - Resetting the CPU")
	if _, err := xDot.SendATCommand(ResetCPUCmd); err != nil {
		log.Println("[ERROR] ResetXDotCPU - Error resetting CPU: " + err.Error())
		return err
	}

	return nil
}

func (xDot *XdotSerialPort) StartSerialDataMode() error {
	log.Println("[INFO] StartSerialDataMode - Starting serial data mode")

	loop := true
	for loop {
		if response, err := xDot.SendATCommand(SerialDataModeCmd); err != nil {
			log.Println("[ERROR] StartSerialDataMode - Error starting serial data mode: " + err.Error())
			return err
		} else {
			if strings.Contains(response, AtCmdConnectText) {
				//If the response contains both CONNECT and OK, this
				//Is a false positive. We need to continue trying
				if !strings.Contains(response, AtCmdSuccessText) {
					loop = false
				}
			}
		}
	}

	return nil
}

func (xDot *XdotSerialPort) StopSerialDataMode() error {
	log.Println("[INFO] StopSerialDataMode - Stopping serial data mode")

	err := errors.New("")
	numTries := 0
	for err != nil && numTries < 10 {
		//Flush serial port before attempting to exit serial data mode
		if err := xDot.FlushSerialPort(); err != nil {
			log.Println("[ERROR] StopSerialDataMode - Error flushing serial port: " + err.Error())
		}

		//Send stop command to the serial port
		//We need to wait 1 second before sending the escape sequence (+++)
		time.Sleep(1 * time.Second)
		if err = xDot.sendStopCommand(); err != nil {
			log.Println("[ERROR] StopSerialDataMode - Error sending stop command: " + err.Error())
			continue
		}

		//Wait a few seconds and then continually try to read from the serial port
		//We need to wait 1 second after sending the escape sequence (+++)
		time.Sleep(1 * time.Second)
		_, err = xDot.readCommandResponse()

		//Ignore "command not found" errors. These indicate the xDot is not in serial data mode
		if err != nil && strings.Contains(err.Error(), "Command not found!") {
			err = nil
			break
		}
		numTries++
	}
	return err
}

func (xDot *XdotSerialPort) sendStopCommand() error {
	log.Println("[INFO] sendStopCommand - Sending stop command")

	//This appears to be the only way to terminate serial data mode reliably.
	//There must be a small amount of time between the send of each "+".
	for i := 0; i < 3; i++ {
		if n, err := xDot.serialPort.Write([]byte("+")); err != nil {
			log.Println("[ERROR] sendStopCommand - Error writing + to serial port: " + err.Error())
			return err
		} else {
			log.Printf("[DEBUG] sendStopCommand - Number of bytes written: %d\n", n)
		}

		time.Sleep(SendStopDelay * time.Millisecond)
	}

	//The carriage return needs to be sent in the event that serial data mode is not active
	time.Sleep(SendStopCarriageReturnDelay * time.Millisecond)
	log.Println("[INFO] sendStopCommand - Sending carriage return")
	if n, err := xDot.serialPort.Write([]byte("\r")); err != nil {
		log.Println("[ERROR] sendStopCommand - Error writing \r to serial port: " + err.Error())
		return err
	} else {
		log.Printf("[DEBUG] sendStopCommand - Number of bytes written: %d\n", n)
	}
	return nil
}

func (xDot *XdotSerialPort) ReadSerialPort() (string, error) {
	var n int
	var err error

	buf := make([]byte, 128)
	incomingData := ""

	for {
		n, err = xDot.serialPort.Read(buf)
		if err != nil { // err will equal io.EOF if there is no data to read
			break
		}
		log.Println("[DEBUG] readSerialPort - Data read: " + string(buf))
		log.Printf("[DEBUG] readSerialPort - Number of bytes read: %d\n", n)
		if n > 0 {
			incomingData += string(buf[:n])
		}
	}

	if n > 0 {
		incomingData += string(buf[:n])
	}

	log.Println("[DEBUG] readSerialPort - Done reading")

	if err != nil {
		if err != io.EOF {
			log.Println("[ERROR] readSerialPort - Error Reading from serial port: " + err.Error())
		}
	}

	return incomingData, err
}

func (xDot *XdotSerialPort) WriteSerialPort(data string) error {
	n, err := xDot.serialPort.Write([]byte(data))
	if err != nil {
		log.Printf("[ERROR] WriteSerialPort - ERROR writing to serial port: %s\n", err.Error())
		return err
	} else {
		log.Printf("[DEBUG] WriteSerialPort - Number of bytes written: %d\n", n)
	}
	return nil
}

func (xDot *XdotSerialPort) FlushSerialPort() error {
	log.Println("[INFO] FlushSerialPort - Flushing serial port")
	if err := xDot.serialPort.Flush(); err != nil {
		log.Println("[ERROR] FlushSerialPort - Error flushing serial port: " + err.Error())
		return err
	}
	return nil
}

func (xDot *XdotSerialPort) ExtractResponseData(atCmd string, cmdResp string) string {

	//Command responses will be in the following format:
	//
	// AT+TXF
	// 915500000
	//
	// OK
	//
	//We need to remove all carriage returns, line feeds,
	//OK, ERROR, and the original command

	//Remove the original command
	parsedResp := strings.Replace(cmdResp, atCmd, "", -1)

	//Remove any OK
	parsedResp = strings.Replace(parsedResp, AtCmdSuccessText, "", -1)

	//Remove any ERROR
	parsedResp = strings.Replace(parsedResp, AtCmdErrorText, "", -1)

	//Remove any CONNECT
	parsedResp = strings.Replace(parsedResp, AtCmdConnectText, "", -1)

	//Remove any carriage returns
	parsedResp = strings.Replace(parsedResp, "\r", "", -1)

	//Remove any line feeds
	parsedResp = strings.Replace(parsedResp, "\n", "", -1)

	log.Println("[DEBUG] extractResponseData - " + parsedResp + " extracted from serial response")
	return parsedResp
}
