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

	"github.com/tarm/serial"
)

type atmodem struct {
	serial	*(serial.Port)
	scanner *(bufio.Scanner)
}

func waitForPort() (atmodem, error) {
	timer := time.NewTicker(4 * time.Second)

	serialConfig := &serial.Config{Name: "/dev/ttyUSB2", Baud: 115200, ReadTimeout: time.Millisecond * 2500}

	log.Printf("Attempting to open modem")
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
	
	for modem.scanner.Scan() {
		msg := modem.scanner.Text()
		log.Printf("msg: \"%s\"\n", msg)
		if msg == "OK" {
			return data, nil
		} else if msg == "ERROR" {
			return data, errors.New("modem reported error")
		} else {
			data = append(data, msg)
		}
	}
	if err := modem.scanner.Err(); err != nil {
		log.Printf("Error reading modem: %s\n", err.Error())
		return data, errors.New("modem read error")
	}
	return data, errors.New("modem exchange unknown termination")
}

func waitForBoot(modem atmodem) {
	timer := time.NewTicker(4 * time.Second)

	log.Printf("Polling modem for responsivity")
	for {
		<- timer.C

		_, err := atCommandExchange(modem, "AT")

		if err != nil {
			log.Printf("Modem AT error: %s\n", err.Error())
		} else {
			return
		}
	}
}

func initLTEGPS(modem atmodem) {
	log.Printf("Initializing modem GPS")

	//To configure:
	// 1) Close any open GPS session
	// 2) Set the output port
	// 3) Set the desired out sentences
	// 4) Configure for 10Hz
	// 5) Start session
	// 6) Enable data stream
	_, err := atCommandExchange(modem, "AT+CGPS=0")
	if err != nil {
		log.Printf("Modem AT error: %s\n", err.Error())
		return
	}

	_, err = atCommandExchange(modem, "AT+CGPSNMEAPORTCFG=3")
	if err != nil {
		log.Printf("Modem AT error: %s\n", err.Error())
		return
	}

	_, err = atCommandExchange(modem, "AT+CGPSNMEA=197119")
	if err != nil {
		log.Printf("Modem AT error: %s\n", err.Error())
		return
	}

	_, err = atCommandExchange(modem, "AT+CGPSNMEARATE=1")
	if err != nil {
		log.Printf("Modem AT error: %s\n", err.Error())
		return
	}

	_, err = atCommandExchange(modem, "AT+CGPS=1")
	if err != nil {
		log.Printf("Modem AT error: %s\n", err.Error())
		return
	}

	_, err = atCommandExchange(modem, "AT+CGPSINFOCFG=1,31")
	if err != nil {
		log.Printf("Modem AT error: %s\n", err.Error())
		return
	}
}

func initLTE() {
	if !globalSettings.LTE_Enabled {
		return
	}

	//Wait for life
	modem, err := waitForPort()
	if err != nil {
		log.Printf("Modem Initialization error: %s\n", err.Error())
		return
	}

	waitForBoot(modem)

	//Check initial configuration for RNDIS
	//Reconfigure if needed

	//Initialize the GPS
	initLTEGPS(modem)

	//Initialize the status reporting
}