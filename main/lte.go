/*
	Copyright (c) 2026 Jon Lovering
	Distributable under the terms of The "BSD New" License
	that can be found in the LICENSE file, herein included
	as part of this header.

	---
	lte.go: Initialization and management of an SIM7600X LTE interface
*/

package main

import (
	"errors"
	"log"
	"time"
	"bufio"
	"strings"
	"strconv"
	"fmt"
	"regexp"

	"github.com/tarm/serial"
)

type atmodem struct {
	serial	*(serial.Port)
	scanner *(bufio.Scanner)
}

func waitForPort() (atmodem, error) {
	timer := time.NewTicker(4 * time.Second)

	serialConfig := &serial.Config{Name: "/dev/ttyUSB2", Baud: 115200, ReadTimeout: time.Millisecond * 2500}

	log.Printf("LTE - Attempting to open modem")
	for {
		<- timer.C
		
		p, err := serial.OpenPort(serialConfig)
		if err != nil {
			log.Printf("LTE - Couldn't open STM7600X Modem: %s\n", err.Error())
			continue
		} else {
			return atmodem{serial: p, scanner: bufio.NewScanner(p)}, nil
		}
	}

	return atmodem{nil, nil}, errors.New("modem wait for port unknown termination")
}

func atCommandExchange(modem atmodem, cmdstring string) ([]string, error) {
	modem.serial.Flush()

	modem.serial.Write([]byte(cmdstring + "\r\n"))

	var data []string
	
	resp_started := false
	for modem.scanner.Scan() {
		msg := strings.TrimSpace(modem.scanner.Text())
		if msg == cmdstring {
			resp_started = true
		} else if msg == "OK" {
			return data, nil
		} else if msg == "ERROR" {
			log.Printf("LTE - cmd: \"%s\" failed\n", cmdstring)
			return data, errors.New("modem reported error")
		} else if resp_started {
			data = append(data, msg)
		} else {
			//Ignore none response lines.
		}
	}
	if err := modem.scanner.Err(); err != nil {
		log.Printf("LTE -  Error reading modem: %s\n", err.Error())
		return nil, errors.New("modem read error")
	}
	return nil, errors.New("modem exchange unknown termination")
}

func atCommandExchange_retry(modem atmodem, cmdstring string, retry int) ([]string, error) {
	for cnt := 0; cnt < retry; cnt++ {
		date, err := atCommandExchange(modem, cmdstring)
		if err != nil {
			log.Printf("LTE - Modem AT error: %s\n", err.Error())
			continue
		} else {
			return date, nil
		}
	}
	return nil, errors.New("LTE - Modem AT errors on retry\n")
}

func atCommandExchangeMatch(modem atmodem, cmdstring string, regex *regexp.Regexp) ([]string, error) {
	data, err := atCommandExchange(modem, cmdstring)
	if err != nil {
		log.Printf("LTE - Modem AT error: %s\n", err.Error())
		return nil, err
	}
	if len(data) >= 1 {
		matches := regex.FindStringSubmatch(data[0])
		if matches != nil {
			return matches, nil
		} else {
			log.Printf("LTE - return \"%s\" could not be matched\n", data[0])	
			return nil, errors.New("LTE - Could not match data from command")
		}
	} else {
		log.Printf("LTE - %s returned 0 length\n", cmdstring)
		return nil, errors.New("LTE - No data returned from command")
	}
}

func atCommandExchangeMatch_retry(modem atmodem, cmdstring string, regex *regexp.Regexp, retry int) ([]string, error) {
	for cnt := 0; cnt < retry; cnt++ {
		matches, err := atCommandExchangeMatch(modem, cmdstring, regex)
		if err != nil {
			log.Printf("LTE - Modem AT error: %s\n", err.Error())
			continue
		} else {
			return matches, nil
		}
	}
	return nil, errors.New("LTE - Modem AT errors on retry\n")
}

func waitForBoot(modem atmodem) {
	timer := time.NewTicker(4 * time.Second)

	log.Printf("LTE - Polling modem for responsivity")
	for {
		<- timer.C

		_, err := atCommandExchange(modem, "AT")

		if err != nil {
			log.Printf("LTE - Modem AT error: %s\n", err.Error())
		} else {
			return
		}
	}
	log.Printf("LTE - Modem online")
}

func initLTEGPS(modem atmodem) error {
	log.Printf("LTE - Initializing modem GPS")

	//To configure:
	// 1) Close any open GPS session
	// 2) Set the output port
	// 3) Set the desired out sentences
	// 4) Configure for 10Hz
	// 5) Start session
	// 6) Enable data stream
	_, err := atCommandExchange(modem, "AT+CGPS=0")
	if err != nil {
		log.Printf("LTE - Modem AT error: %s\n", err.Error())
		return err
	}

	_, err = atCommandExchange(modem, "AT+CGPSNMEAPORTCFG=3")
	if err != nil {
		log.Printf("LTE - Modem AT error: %s\n", err.Error())
		return err
	}

	_, err = atCommandExchange(modem, "AT+CGPSNMEA=197119")
	if err != nil {
		log.Printf("LTE - Modem AT error: %s\n", err.Error())
		return err
	}

	_, err = atCommandExchange(modem, "AT+CGPSNMEARATE=1")
	if err != nil {
		log.Printf("LTE - Modem AT error: %s\n", err.Error())
		return err
	}

	_, err = atCommandExchange(modem, "AT+CGPS=1")
	if err != nil {
		log.Printf("LTE - Modem AT error: %s\n", err.Error())
		return err
	}

	_, err = atCommandExchange(modem, "AT+CGPSINFOCFG=1,31")
	if err != nil {
		log.Printf("LTE - Modem AT error: %s\n", err.Error())
		return err
	}

	return nil
}

func updateLTEStatus(modem atmodem) {
	timer := time.NewTicker(10 * time.Second)

	reCSQ := regexp.MustCompile(`\+CSQ: (\d+),(\d+)`)
	reCOPS := regexp.MustCompile(`\+COPS: (\d+),(\d+),"(\S+)",(\d+)`)
	reCPSI := regexp.MustCompile(`\+CPSI: (\S+),(\S+),\S+,\S+,\S+,\S+,\S+,\S+,\S+,\S+,\S+,\S+,\S+,\S+`)

	for {
		<- timer.C

		matches, err := atCommandExchangeMatch(modem, "AT+CSQ", reCSQ)
		if err != nil {
			log.Printf("LTE - Modem AT error: %s\n", err.Error())
		}
		if matches != nil {
			rssi_raw, err := strconv.ParseInt(matches[1], 10, 16)
			if err != nil {
				log.Printf("LTE - Modem could not parse to int: \"%s\", %s\n", matches[1], err.Error())
			} else {
				if rssi_raw == 0 {
					globalStatus.LTE_SignalStrength = "<-113"
				} else if rssi_raw < 31 {
					globalStatus.LTE_SignalStrength = fmt.Sprintf("%d", -113 + rssi_raw * 2)
				} else if rssi_raw == 31 {
					globalStatus.LTE_SignalStrength = ">-51"
				} else if rssi_raw > 99 && rssi_raw < 191 {
					globalStatus.LTE_SignalStrength = fmt.Sprintf("%d", -116 + rssi_raw)
				} else if rssi_raw == 191 {
					globalStatus.LTE_SignalStrength = ">-25"
				} else if rssi_raw == 99 || rssi_raw == 199 {
					globalStatus.LTE_SignalStrength = "Unknown"
				} else {
					log.Printf("LTE - Modem unexpected signal strength: %d\n", rssi_raw)
				}
			}
		}

		matches, err = atCommandExchangeMatch(modem, "AT+COPS?", reCOPS)
		if err != nil {
			log.Printf("LTE - Modem AT error: %s\n", err.Error())
		}
		if matches != nil {
			// If the re matches, then it must have all the results
			globalStatus.LTE_Network = matches[3]
		}

		matches, err = atCommandExchangeMatch(modem, "AT+CPSI?", reCPSI)
		if err != nil {
			log.Printf("LTE - Modem AT error: %s\n", err.Error())
		}
		if (matches != nil) {
			// If the re matches, then it must have all the results
			globalStatus.LTE_Mode = fmt.Sprintf("%s - %s", matches[1], matches[2])
		}
	}
}

func configureModem(modem atmodem) error {
	reCICCID := regexp.MustCompile(`\+ICCID: (\d{20})`)
	reCSPN := regexp.MustCompile(`\+CSPN: "(\S*)",\S+`)
	reSIMEI := regexp.MustCompile(`\+SIMEI: (\d{15})`)
	reCUSBPIDSWITCH := regexp.MustCompile(`\+CUSBPIDSWITCH: (\S+)`)
	
	matches, err := atCommandExchangeMatch(modem, "AT+CICCID", reCICCID)
	if err != nil {
		log.Printf("LTE - Modem AT error: %s\n", err.Error())
		return err
	}
	globalStatus.LTE_ICCID = matches[1]

	matches, err = atCommandExchangeMatch(modem, "AT+CSPN?", reCSPN)
	if err != nil {
		log.Printf("LTE - Modem AT error: %s\n", err.Error())
		return err
	}
	globalStatus.LTE_SPN = matches[1]
	
	matches, err = atCommandExchangeMatch(modem, "AT+SIMEI?", reSIMEI)
	if err != nil {
		log.Printf("LTE - Modem AT error: %s\n", err.Error())
		return err
	}
	globalStatus.LTE_IMEI = matches[1]

	matches, err = atCommandExchangeMatch(modem, "AT+CUSBPIDSWITCH?", reCUSBPIDSWITCH)
	if err != nil {
		log.Printf("LTE - Modem AT error: %s\n", err.Error())
		return err
	}
	pid_data := matches[1]
	if pid_data != "9011" {
		log.Printf("LTE - LTE Modem not in RNDIS mode, attempting to reconfigure: \"%s\"\n", pid_data)

		_, err = atCommandExchange(modem, "AT+CUSBPIDSWITCH=9011,1,1")
		if err != nil {
			log.Printf("LTE - Modem AT error: %s\n", err.Error())
			return err
		}

		_, err = atCommandExchange(modem, fmt.Sprintf("AT+CGDCONT=1,\"IPV4V6\",\"%s\"",globalSettings.LTE_APN))
		if err != nil {
			log.Printf("LTE - Modem AT error: %s\n", err.Error())
			return err
		}

		_, err = atCommandExchange(modem, fmt.Sprintf("AT+CGDCONT=6,\"IPV4V6\",\"%s\"",globalSettings.LTE_APN))
		if err != nil {
			log.Printf("LTE - Modem AT error: %s\n", err.Error())
			return err
		}

		_, err = atCommandExchange(modem, "AT+CRESET")
		if err != nil {
			log.Printf("LTE - Modem AT error: %s\n", err.Error())
			return err
		}

		time.Sleep(5)
		waitForBoot(modem)
	}

	return nil
}

func initLTE() {
	globalStatus.LTE_Network = "N/A"
	globalStatus.LTE_SignalStrength = "N/A"
	globalStatus.LTE_Mode = "N/A"
	globalStatus.LTE_ICCID = "N/A"
	globalStatus.LTE_SPN = "N/A"
	globalStatus.LTE_IMEI = "N/A"

	if !globalSettings.LTE_Enabled {
		return
	}

	//Wait for life
	modem, err := waitForPort()
	if err != nil {
		log.Printf("LTE - Modem Initialization error: %s\n", err.Error())
		return
	}

	configured := false
	gpsInit := false

	timer := time.NewTicker(2 * time.Second)

	// Loop and try to configure the modem. We should only get here if there appears
	// to be a modem port. The configuration could fail, so we will keep trying if
	// it does.
	for (!(configured && gpsInit)) {
		<- timer.C

		waitForBoot(modem)
		//Check initial configuration for RNDIS
		//Reconfigure if needed
		if (!configured) {
			err := configureModem(modem)
			if err == nil {
				configured = true
			} else {
				continue
			}
		}

		if (!gpsInit) {
			//Initialize the GPS
			err := initLTEGPS(modem)
			if err == nil {
				gpsInit = true
			} else {
				continue
			}
		}
	}

	//Initialize the status reporting
	go updateLTEStatus(modem)
}