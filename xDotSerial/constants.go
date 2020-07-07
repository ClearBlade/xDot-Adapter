package xDotSerial

//Network join modes
const ManualConfigMode = "0"
const OtaJoinMode = "1"
const AutoOtaJoinMode = "2"
const PeerToPeerMode = "3"

//Device Classes
const DeviceClassA = "A"
const DeviceClassB = "B"
const DeviceClassC = "C"

// Public Network Modes
const PrivateMTSNetworkMode = "0"
const PublicLoRaWANNetworkMode = "1"
const PrivateLoRaWANNetworkMode = "2"

//AT Command assistance can be found by referencing the AT Command reference guide:
//
// http://wwwmultitechcom/documents/publications/manuals/s000643pdf

// BEGIN AT COMMANDS

//General AT Commands
const AttnCmd = "AT"
const RequestIDCmd = "ATI"
const ResetCPUCmd = "ATZ"
const DisableEchoCmd = "ATE0"
const EnableEchoCmd = "ATE1"
const DisableVerboseCmd = "ATV0"
const EnableVerboseCmd = "ATV1"
const DisableHWFlowControlCmd = "AT&K0"
const EnableHWFlowControlCmd = "AT&K3"
const ResetFactoryDefaultsCmd = "AT&F"
const SaveConfigurationCmd = "AT&W"
const WakePinCmd = "AT+WP"
const SerialSpeedCmd = "AT+IPR"
const DebugSerialSpeedCmd = "AT+DIPR"
const DebugLogLevelCmd = "AT+LOG"

//Network Management commands
const DeviceIDCmd = "AT+DI"
const DefaultFrequencyBandCmd = "AT+DFREQ"
const FrequencyBandCmd = "AT+FREQ"
const FrequencySubBandCmd = "AT+FSB"
const PublicNetworkModeCmd = "AT+PN"
const JoinByteOrderCmd = "AT+JBO"
const NetworkJoinModeCmd = "AT+NJM"
const NetworkJoinCmd = "AT+JOIN"
const NetworkJoinRetriesCmd = "AT+JR"
const NetworkJoinDelayCmd = "AT+JD"

//Over-the-air Activation Commands
const NetworkIdCmd = "AT+NI"
const NetworkKeyCmd = "AT+NK"
const NetworkAesEncryptionCmd = "AT+ENC"

//Manual Activation Commands
const NetworkAddrCmd = "AT+NA"
const NetworkSessionKeyCmd = "AT+NSK"
const NetworkDataKeyCmd = "AT+DSK"
const NetworkUplinkCounterCmd = "AT+ULC"
const NetworkDownlinkCounterCmd = "AT+DLC"

//Ensuring Network Connectivity Commands
const NetworkJoinStatusCmd = "AT+NJS"
const PingCmd = "AT+PING"
const RequireAcknowledgementCmd = "AT+ACK"
const NetworkLinkCheckCmd = "AT+NLC"
const NetworkLinkCheckCountCmd = "AT+LCC"
const LinkCheckThresholdCmd = "AT+LCT"

//Preserving, Saving, and Restoring Sessions
const SaveNetworkSessionCmd = "AT+SS"
const RestoreNetworkSessionCmd = "AT+RS"
const PreserveNetworkSessionCmd = "AT+PS"

//Channels and Duty Cycles Commands
const ChannelMaskCmd = "AT+CHM"
const TransmitChannelCmd = "AT+TXCH"
const ListenBeforeTalkCmd = "AT+LBT"
const TransmitNextCmd = "AT+TXN"
const TimeOnAirCmd = "AT+TOA"

//Configuration
const InjectMacCmd = "AT+MAC"
const SettingsAndStatusCmd = "AT&V"
const DeviceClassCmd = "AT+DC"
const ApplicationPortCmd = "AT+AP"
const TransmitPowerCmd = "AT+TXP"
const TransmitInvertedCmd = "AT+TXI"
const ReceiveSignalInvertedCmd = "AT+RXI"
const ReceiveDelayCmd = "AT+RXD"
const ForwardErrorCorrectionCmd = "AT+FEC"
const CyclicalRedundancyCheckCmd = "AT+CRC"
const AdaptiveDataRateCmd = "AT+ADR"
const TransmissionDataRateCmd = "AT+TXDR"
const SessionDataRateCmd = "AT+SDR"
const RepeatPacketCmd = "AT+REP"

//Sending Packets
const SendDataCmd = "AT+SEND"
const SendBinaryCmd = "AT+SENDB"

//Receiving Packets
const ReceiveDataONceCmd = "AT+RECV"
const ReceiveOutputCmd = "AT+RXO"
const DataPendingCmd = "AT+DP"
const TransmitWaitCmd = "AT+TXW"

//Statistics
const ResetStatisticsCmd = "AT&R"
const SettingsAndStatisticsCmd = "AT&S"
const SignalStrengthCmd = "AT+RSSI"
const SignalToNoiseRatioCmd = "AT+SNR"

//Serial Data Mode
const SerialDataModeCmd = "AT+SD"
const SerialDataStartupModeCmd = "AT+SMODE"
const SerialDataClearOnErrorCmd = "AT+SDCE"

//Power Management
const SleepModeCmd = "AT+SLEEP"
const WakeModeCmd = "AT+WM"
const WakeInterval = "AT+WI"
const WakeDelayCmd = "AT+WD"
const WakeTimeoutCmd = "AT+WTO"
const AntennaGainCmd = "AT+ANT"

//Testing and Compliance
const ReceiveDataRateCmd = "AT+RXDR"
const ReceiveFrequencyCmd = "AT+RXF"
const ReceiveContinuouslyCmd = "AT+RECVC"
const SendOnIntervalCmd = "AT+SENDI"
const TransmissionFrequencyCmd = "AT+TXF"

// END OF AT COMMANDS

const AtCmdSuccessText = "OK\r\n"
const AtCmdErrorText = "ERROR\r\n"
const AtCmdConnectText = "CONNECT\r\n"

const SendStopDelay = 250               //milliseconds
const SendStopCarriageReturnDelay = 750 //milliseconds
