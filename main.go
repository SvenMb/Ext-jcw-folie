package main

//go:generate go-bindata data/

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/chzyer/readline"
	"github.com/tarm/serial"
)

var (
	port = flag.String("p", "", "serial port (usually /dev/tty* or COM*)")
	baud = flag.Int("b", 115200, "serial baud rate")

	tasks      sync.WaitGroup
	tty        *serial.Port
	console    *readline.Instance
	serialIn   = make(chan []byte, 0)
	commandOut = make(chan string, 0)
)

func main() {
	flag.Parse()

	tasks.Add(1)
	go consoleTask()

	go readSerial()

	go func() {
		for data := range serialIn {
			os.Stdout.Write(data)
		}
	}()

	go func() {
		for data := range commandOut {
			tty.Write([]byte(data + "\r"))
		}
	}()

	tasks.Wait()
}

func readSerial() {
	for {
		var err error
		config := serial.Config{Name: *port, Baud: *baud}
		tty, err = serial.OpenPort(&config)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// by using readline's Stdout, we can force re-display of current input
		fmt.Fprintln(console.Stdout(), "[connected]")
		for {
			data := make([]byte, 250)
			n, err := tty.Read(data)
			if err == io.EOF {
				break
			}
			check(err)
			serialIn <- data[:n]
		}
		fmt.Print("\n[disconnected] ")

		tty.Close()
	}
}

// insertCRs is used to insert lost CRs when readline is active
func insertCRs(out *os.File) *os.File {
	readFile, writeFile, err := os.Pipe()
	check(err)

	go func() {
		defer readFile.Close()
		for {
			data := make([]byte, 250)
			n, err := readFile.Read(data)
			if err != nil {
				break
			}
			data = bytes.Replace(data[:n], []byte("\n"), []byte("\r\n"), -1)
			out.Write(data)
		}
	}()

	return writeFile
}

func consoleTask() {
	defer tasks.Done()

	if readline.IsTerminal(1) {
		os.Stdout = insertCRs(os.Stdout)
	}
	if readline.IsTerminal(2) {
		os.Stderr = insertCRs(os.Stderr)
	}

	var err error
	config := readline.Config{
		UniqueEditLine: true,
		Stdout:         os.Stdout,
	}
	console, err = readline.NewEx(&config)
	check(err)
	defer console.Close()

	for {
		line, err := console.Readline()
		if err != nil {
			break
		}
		commandOut <- line
	}
}

func check(e error) {
	if e != nil {
		log.Fatal(e)
	}
}
