package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.bug.st/serial.v1"
)

var (
	port = flag.String("p", "", "serial port (COM*, /dev/cu.*, or /dev/tty*)")
	baud = flag.Int("b", 115200, "serial baud rate")

	tty       serial.Port
	openBlock = make(chan string)
)

// SerialConnect opens and re-opens a serial port and feeds the receive channel.
func SerialConnect() {
	if *port == "" {
		commandSend <- "<open>"
		*port = <-openBlock
	}

	for {
		conn, err := serial.Open(*port, &serial.Mode{
			BaudRate: *baud,
		})
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		tty = conn

		// use readline's Stdout to force re-display of current input
		fmt.Fprintf(console.Stdout(), "[connected: %s]\n", *port)
		for {
			data := make([]byte, 250)
			n, err := tty.Read(data)
			if err == io.EOF || err == syscall.ENXIO {
				break
			}
			check(err)
			serialRecv <- data[:n]
		}
		fmt.Print("\n[disconnected] ")

		tty.Close()
		tty = nil
	}
}

// SerialDispatch handles all incoming and outgoing serial data.
func SerialDispatch() {
	// outbound serial will be slowed down just a tad for Mecrisp/USB
	go func() {
		for data := range serialSend {
			for len(data) > 0 {
				out := data
				if len(out) > 60 {
					out = out[:60] // send as separate chunks of under 64 bytes
				}
				data = data[len(out):]

				if tty == nil { // avoid write-while-closed panics
					fmt.Printf("[can't write: %s]\n", *port)
				} else if _, err := tty.Write(out); err != nil {
					fmt.Printf("[write error: %s]\n", *port)
				} else if len(data) > 0 {
					// when chunked, add a brief delay to force separate sends
					time.Sleep(2 * time.Millisecond)
				}
			}
		}
	}()

	for {
		select {

		case data := <-serialRecv:
			if *verbose {
				fmt.Printf("recv: %q\n", data)
			}
			os.Stdout.Write(data)

		case line := <-commandSend:
			if !SpecialCommand(line) {
				data := []byte(line + "\r")
				if *verbose {
					fmt.Printf("send: %q\n", data)
				}
				serialSend <- data
			}
		}
	}
}

// SpecialCommand recognises and handles certain commands in a different way.
func SpecialCommand(line string) bool {
	cmd := strings.Split(line, " ")
	if len(cmd) > 0 {
		switch cmd[0] {

		case "<open>":
			// TODO can't be typed in to re-open, only usable on startup
			WrappedOpen(cmd)

		case "!s", "!send":
			fmt.Println(line)
			WrappedSend(cmd)

		case "!u", "!upload":
			fmt.Println(line)
			WrappedUpload(cmd)

		default:
			return false
		}
	}
	return true
}

func WrappedOpen(argv []string) {
	allPorts, err := serial.GetPortsList()
	check(err)

	var ports []string
	for _, p := range allPorts {
		if !strings.HasPrefix(p, "/dev/tty.") {
			ports = append(ports, p)
		}
	}

	fmt.Println("Select a serial port:")
	for i, p := range ports {
		fmt.Println("  ", i+1, "=", p)
	}
	reply := <-commandSend
	fmt.Println(reply)

	sel, _ := strconv.Atoi(reply)

	// nasty way to quit on index errors, since there's no other useful option!
	defer func() {
		if e := recover(); e != nil {
			done <- nil // forces quit without producing an error message
		}
	}()

	openBlock <- ports[sel-1]
}

func WrappedSend(argv []string) {
	if len(argv) == 1 {
		fmt.Printf("Usage: %s <filename>\n", argv[0])
		return
	}
	if !SendFile(argv[1]) {
		fmt.Println("Send failed.")
	}
}

func WrappedUpload(argv []string) {
	if len(argv) == 1 {
		fmt.Println("Built-in firmware images:")
		names, _ := AssetDir("data")
		sort.Strings(names)
		for _, name := range names {
			info, _ := AssetInfo("data/" + name)
			fmt.Printf("  %-15s %5db\n", name, info.Size())
		}
		return
	}

	// try built-in images first
	data, err := Asset("data/" + argv[1])
	if err != nil {
		// else try opening the arg as file
		f, err := os.Open(argv[1])
		if err != nil {
			fmt.Println(err)
			return
		}
		defer f.Close()

		data, err = ioutil.ReadAll(f)
		check(err)
	}

	// temporarily switch to even parity during upload
	tty.SetMode(&serial.Mode{BaudRate: *baud, Parity: serial.EvenParity})
	defer tty.SetMode(&serial.Mode{BaudRate: *baud})

	Uploader(data)
}
