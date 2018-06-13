package xDotSerial

import (
	"errors"
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
	}

	resp, err := xDot.readCommandResponse()
	if err != nil {
		return "", err
	}

	log.Println("[DEBUG] SendATCommand - Returning AT command response: " + string(resp[:]))
	return xDot.extractResponseData(cmd, string(resp[:])), nil
}

func (xDot *XdotSerialPort) readCommandResponse() (string, error) {
	sleepTime := 2500 * time.Millisecond

	// Every AT command will return either "OK\r\n", "ERROR\r\n", or "CONNECT\r\n" (In the
	// case of enabling serial data mode)
	// Read from the response from the serial port until either "OK\r\n" or "ERROR\r\n" are returned

	log.Println("[DEBUG] readCommandResponse - Reading response from serial port")

	resp := ""
	numTries := 1

	buff, err := xDot.ReadSerialPort(true)
	if err != nil {
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

		buff, err = xDot.ReadSerialPort(true)
		if err != nil {
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
	if strings.Contains(string(resp[:]), AtCmdErrorText) {
		log.Println("[DEBUG] readCommandResponse - Error received executing AT command: " + string(resp[:]))
		return "", errors.New(string(resp[:]))
	}

	log.Println("[DEBUG] readCommandResponse - Returning AT command response: " + string(resp[:]))
	return string(resp[:]), nil
}

func (xDot *XdotSerialPort) GetDeviceID() (string, error) {
	log.Println("[DEBUG] GetDeviceID - Retrieving device ID")
	var deviceID string
	var err error
	if deviceID, err = xDot.SendATCommand(DeviceIDCmd); err != nil {
		log.Println("[ERROR] GetDeviceID - Error retrieving network join mode: " + err.Error())
		return "", err
	}
	return deviceID, nil
}

func (xDot *XdotSerialPort) GetDeviceClass() (string, error) {
	log.Println("[DEBUG] GetDeviceClass - Retrieving device class")
	var devClass string
	var err error
	if devClass, err = xDot.SendATCommand(DeviceClassCmd); err != nil {
		log.Println("[ERROR] GetDeviceClass - Error retrieving device class: " + err.Error())
		return "", err
	}
	return devClass, nil
}

func (xDot *XdotSerialPort) SetDeviceClass(devClass string) (bool, error) {
	log.Println("[DEBUG] SetDeviceClass - Begin setting device class to " + devClass)

	//Retrieve the current value. Don't update it if we don't have to
	currentValue, err := xDot.GetDeviceClass()

	log.Println("[DEBUG] SetDeviceClass - Current value = " + currentValue)
	log.Println("[DEBUG] SetDeviceClass - Value to set = " + devClass)

	if err != nil || !strings.Contains(currentValue, devClass) {
		if err != nil {
			log.Println("[ERROR] SetDeviceClass - Error retrieving device class: " + err.Error())
			return false, err
		} else {
			if _, err := xDot.SendATCommand(DeviceClassCmd + "=" + devClass); err != nil {
				log.Println("[ERROR] SetDeviceClass - Error setting device class: " + err.Error())
				return false, err
			}
			log.Println("[DEBUG] SetDeviceClass - Device class set to " + devClass)
			return true, nil
		}
	}

	return false, nil
}

func (xDot *XdotSerialPort) GetNetworkJoinMode() (string, error) {
	log.Println("[DEBUG] GetNetworkJoinMode - Retrieving network join mode")
	var joinMode string
	var err error
	if joinMode, err = xDot.SendATCommand(NetworkJoinModeCmd); err != nil {
		log.Println("[ERROR] GetNetworkJoinMode - Error retrieving network join mode: " + err.Error())
		return "", err
	}
	return joinMode, nil
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
			log.Println("[DEBUG] SetNetworkJoinMode - Network join mode set to " + mode)
			return true, nil
		}
	}

	return false, nil
}

func (xDot *XdotSerialPort) GetNetworkAddress() (string, error) {
	log.Println("[DEBUG] getNetworGetNetworkAddresskAddress - Retrieving network address")
	var address string
	var err error
	if address, err = xDot.SendATCommand(NetworkAddrCmd); err != nil {
		log.Println("[ERROR] GetNetworkAddress - Error retrieving network address: " + err.Error())
		return "", err
	}
	return address, nil
}

func (xDot *XdotSerialPort) SetNetworkAddress(address string) (bool, error) {
	log.Println("[DEBUG] SetNetworkAddress - Setting network address")

	//Retrieve the current value. Don't update it if we don't have to
	currentValue, err := xDot.GetNetworkAddress()

	log.Println("[DEBUG] SetNetworkAddress - Current value = " + currentValue)
	log.Println("[DEBUG] SetNetworkAddress - Value to set = " + address)

	if err != nil || !strings.Contains(currentValue, address) {
		if err != nil {
			log.Println("[DEBUG] SetNetworkAddress - Error retrieving network address: " + err.Error())
			return false, err
		} else {
			if _, err := xDot.SendATCommand(NetworkAddrCmd + "=" + address); err != nil {
				log.Println("[ERROR] SetNetworkAddress - Error setting network address: " + err.Error())
				return false, err
			}
			log.Println("[DEBUG] SetNetworkAddress - Network session key set to " + address)
			return true, nil
		}
	}

	return false, nil
}

func (xDot *XdotSerialPort) GetNetworkSessionKey() (string, error) {
	log.Println("[DEBUG] GetNetworkSessionKey - Retrieving network session key")
	var sessionKey string
	var err error
	if sessionKey, err = xDot.SendATCommand(NetworkSessionKeyCmd); err != nil {
		log.Println("[ERROR] GetNetworkSessionKey - Error setting network session key: " + err.Error())
		return "", err
	}
	return sessionKey, nil
}

func (xDot *XdotSerialPort) SetNetworkSessionKey(sessionKey string) (bool, error) {
	log.Println("[DEBUG] SetNetworkSessionKey - Setting network session key")
	//Retrieve the current value. Don't update it if we don't have to
	currentValue, err := xDot.GetNetworkSessionKey()

	//Replace periods with colons in the returned value
	currentValue = strings.Replace(currentValue, ".", ":", -1)

	log.Println("[DEBUG] SetNetworkSessionKey - Current value = " + currentValue)
	log.Println("[DEBUG] SetNetworkSessionKey - Value to set = " + sessionKey)

	if err != nil || !strings.Contains(currentValue, sessionKey) {
		if err != nil {
			log.Println("[ERROR] SetNetworkSessionKey - Error retrieving network session key: " + err.Error())
			return false, err
		} else {
			if _, err := xDot.SendATCommand(NetworkSessionKeyCmd + "=" + sessionKey); err != nil {
				log.Println("[ERROR] SetNetworkSessionKey - Error setting network session key: " + err.Error())
				return false, err
			}
			log.Println("[DEBUG] SetNetworkSessionKey - Network session key set to " + sessionKey)
			return true, nil
		}
	}

	return false, nil
}

func (xDot *XdotSerialPort) GetDataSessionKey() (string, error) {
	log.Println("[DEBUG] GetDataSessionKey - Retrieving data session key")
	var sessionKey string
	var err error
	if sessionKey, err = xDot.SendATCommand(NetworkDataKeyCmd); err != nil {
		log.Println("[ERROR] GetDataSessionKey - Error retrieving data session key" + err.Error())
		return "", err
	}
	return sessionKey, nil
}

func (xDot *XdotSerialPort) SetDataSessionKey(dataKey string) (bool, error) {
	log.Println("[DEBUG] SetDataSessionKey - Setting data session key")
	//Retrieve the current value. Don't update it if we don't have to
	currentValue, err := xDot.GetDataSessionKey()

	//Replace periods with colons in the returned value
	currentValue = strings.Replace(currentValue, ".", ":", -1)

	log.Println("[DEBUG] SetDataSessionKey - Current value = " + currentValue)
	log.Println("[DEBUG] SetDataSessionKey - Value to set = " + dataKey)

	if err != nil || !strings.Contains(currentValue, dataKey) {
		if err != nil {
			log.Println("[ERROR] SetDataSessionKey - Error retrieving data session key")
			return false, err
		} else {
			if _, err := xDot.SendATCommand(NetworkDataKeyCmd + "=" + dataKey); err != nil {
				log.Println("[ERROR] SetDataSessionKey - Error setting data session key")
				return false, err
			}
			log.Println("[DEBUG] SetDataSessionKey - Data session key set to " + dataKey)
			return true, nil
		}
	}

	return false, nil
}

func (xDot *XdotSerialPort) GetDataRate() (string, error) {
	log.Println("[DEBUG] GetDataRate - Retrieving data transmission rate")
	var dataRate string
	var err error
	if dataRate, err = xDot.SendATCommand(TransactionDataRateCmd); err != nil {
		log.Println("[ERROR] GetDataRate - Error retrieving data transmission rate")
		return "", err
	}
	return dataRate, nil
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
			if _, err := xDot.SendATCommand(TransactionDataRateCmd + "=" + dataRate); err != nil {
				log.Println("[DEBUG] SetDataRate - Error setting data transmission rate")
				return false, err
			}
			log.Println("[DEBUG] SetDataRate - Data transmission rate set to " + dataRate)
			return true, nil
		}
	}

	return false, nil
}

func (xDot *XdotSerialPort) GetFrequency() (string, error) {
	log.Println("[DEBUG] GetFrequency - Retrieving frequency")
	var frequency string
	var err error
	if frequency, err = xDot.SendATCommand(TransactionFrequencyCmd); err != nil {
		log.Println("[ERROR] GetFrequency - Error retrieving frequency: " + err.Error())
		return "", err
	}
	return frequency, nil
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
			if _, err := xDot.SendATCommand(TransactionFrequencyCmd + "=" + freq); err != nil {
				log.Println("[ERROR] SetFrequency - Error setting data transmission frequency: " + err.Error())
				return false, err
			}
			log.Println("[DEBUG] SetFrequency - Data transmission frequency set to " + freq)
			return true, nil
		}
	}

	return false, nil
}

func (xDot *XdotSerialPort) SaveConfiguration() error {
	log.Println("[DEBUG] SaveConfiguration - Saving xDot configuration")
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

	//Wait a few seconds and then continually try to open the port
	time.Sleep(3 * time.Second)

	err := xDot.OpenSerialPort()
	var numTries = 1
	for err != nil && numTries <= 5 {
		log.Println("[ERROR] ResetXDotCPU - Error opening serial port: " + err.Error())
		log.Println("[DEBUG] ResetXDotCPU - Unable to open serial port, waiting 500 milliseconds")
		time.Sleep(500 * time.Millisecond)

		err = xDot.OpenSerialPort()
		numTries++
	}

	if numTries > 5 {
		log.Println("[ERROR] ResetXDotCPU - Unable to open serial port after 2500 milliseconds")
		return errors.New("Unable to open serial port after 2500 milliseconds")
	}

	return nil
}

func (xDot *XdotSerialPort) StartSerialDataMode() error {
	log.Println("[DEBUG] StartSerialDataMode - Starting serial data mode")
	if _, err := xDot.SendATCommand(SerialDataModeCmd); err != nil {
		log.Println("[ERROR] StartSerialDataMode - Error starting serial data mode: " + err.Error())
		return err
	}
	return nil
}

func (xDot *XdotSerialPort) StopSerialDataMode() error {
	log.Println("[DEBUG] StopSerialDataMode - Stopping serial data mode")

	err := errors.New("")
	for err != nil {
		//Flush serial port before attempting to exit serial data mode
		if err := xDot.FlushSerialPort(); err != nil {
			log.Println("[ERROR] subscribeWorker - Error flushing serial port: " + err.Error())
		}

		//Send stop command to the serial port
		if err = xDot.sendStopCommand(); err != nil {
			log.Println("[ERROR] StopSerialDataMode - Error sending stop command: " + err.Error())
			continue
		}

		//Wait a few seconds and then continually try to read from the serial port
		time.Sleep(1 * time.Second)

		_, err = xDot.readCommandResponse()

		//Ignore "command not found" errors. These indicate the xDot is not in serial data mode
		if err != nil && strings.Contains(err.Error(), "Command not found!") {
			break
		}
	}

	return nil
}

func (xDot *XdotSerialPort) sendStopCommand() error {
	log.Println("[Info] sendStopCommand - Sending stop command")

	// if _, err := xDot.serialPort.Write([]byte(SerialDataModeStopCmd)); err != nil {
	// 	log.Println("[ERROR] sendStopCommand - Error writing SerialDataModeStopCmd to serial port: " + err.Error())
	// 	return err
	// }

	//TODO - Only time will tell, but this appears to be the only way to terminate
	//serial data mode reliably. It appears that there must be a small amount of time
	//between the send of each "+". In addition sending a carriage return screws it up.
	for i := 0; i < 3; i++ {
		if n, err := xDot.serialPort.Write([]byte("+")); err != nil {
			log.Println("[ERROR] sendStopCommand - Error writing + to serial port: " + err.Error())
			return err
		} else {
			log.Printf("[DEBUG] sendStopCommand - Number of bytes written: %d\n", n)
		}

		time.Sleep(50 * time.Millisecond)
	}

	// log.Println("[Info] sendStopCommand - Sending carriage return")
	// if _, err := xDot.serialPort.Write([]byte("\r")); err != nil {
	// 	log.Println("[ERROR] sendStopCommand - Error writing \r to serial port: " + err.Error())
	// 	return err
	// }
	return nil
}

func (xDot *XdotSerialPort) ReadSerialPort(ignoreEOF bool) (string, error) {
	buff := make([]byte, 128)

	n, err := xDot.serialPort.Read(buff)

	if err != nil {
		if (ignoreEOF && !strings.Contains(err.Error(), "EOF")) || !ignoreEOF {
			log.Println("[ERROR] readSerialPort - Error Reading from serial port: " + err.Error())
			return "", err
		}
	}

	log.Printf("[DEBUG] readSerialPort - Number of bytes read: %d\n", n)
	return string(buff[:n]), nil
}

func (xDot *XdotSerialPort) WriteSerialPort(data string) error {
	_, err := xDot.serialPort.Write([]byte(data))
	if err != nil {
		log.Printf("[ERROR] WriteSerialPort - ERROR writing to serial port: %s\n", err.Error())
		return err
	}
	return nil
}

func (xDot *XdotSerialPort) FlushSerialPort() error {
	if err := xDot.serialPort.Flush(); err != nil {
		log.Println("[ERROR] FlushSerialPort - Error flushing serial port: " + err.Error())
		return err
	}
	return nil
}

func (xDot *XdotSerialPort) extractResponseData(atCmd string, cmdResp string) string {

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
	parsedResp = strings.Replace(parsedResp, "OK", "", -1)

	//Remove any ERROR
	parsedResp = strings.Replace(parsedResp, "ERROR", "", -1)

	//Remove any CONNECT
	parsedResp = strings.Replace(parsedResp, "CONNECT", "", -1)

	//Remove any carriage returns
	parsedResp = strings.Replace(parsedResp, "\r", "", -1)

	//Remove any line feeds
	parsedResp = strings.Replace(parsedResp, "\n", "", -1)

	log.Println("[DEBUG] extractResponseData - " + parsedResp + " extracted from serial response")
	return parsedResp
}
