package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"go.bug.st/serial.v1"
)

var (
	port = flag.String("p", "", "serial port (COM*, /dev/cu.*, or /dev/tty*)")
	baud = flag.Int("b", 115200, "serial baud rate")

	tty serial.Port
)

// SerialConnect opens and re-opens a serial port and feeds the receive channel.
func SerialConnect() {
	for {
		var err error
		tty, err = serial.Open(*port, &serial.Mode{
			BaudRate: *baud,
		})
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// use readline's Stdout to force re-display of current input
		fmt.Fprintln(console.Stdout(), "[connected]")
		var data [250]byte
		for {
			n, err := tty.Read(data[:])
			if err == io.EOF {
				break
			}
			check(err)
			serialRecv <- data[:n]
		}
		fmt.Print("\n[disconnected] ")

		tty.Close()
	}
}

// SerialDispatch handles all incoming and outgoing serial data.
func SerialDispatch() {
	for {
		select {

		case data := <-serialRecv:
			os.Stdout.Write(data)

		case cmd := <-commandSend:
			// FIXME need a way to recover from write-while-closed panics
			if !SpecialCommand(cmd) {
				tty.Write([]byte(cmd + "\r"))
			}
		}
	}
}

// SpecialCommand recognises and handles certain commands in a different way.
func SpecialCommand(line string) bool {
	cmd := strings.Split(line, " ")
	if len(cmd) > 0 {
		switch cmd[0] {

		case "upload":
			WrappedUpload(cmd[1:])

		default:
			return true
		}
	}
	return false
}

func WrappedUpload(argv []string) {
	// switch to even parity during upload only
	tty.SetMode(&serial.Mode{
		BaudRate: *baud,
		Parity:   serial.EvenParity,
	})
	defer tty.SetMode(&serial.Mode{
		BaudRate: *baud,
	})

	Uploader(MustAsset("data/mecrisp.bin"), tty)
}
