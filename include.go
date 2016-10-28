package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path"
	"strings"
	"time"
)

var callCount int

// IncludeFile sends out one file, expanding embdded includes as needed.
func IncludeFile(name string, level int) bool {
	f, err := os.Open(name)
	if err != nil {
		fmt.Println(err)
		return false
	}
	defer f.Close()

	currFile := path.Base(name)
	currLine := 0
	if level == 0 {
		callCount = 0
	}
	callCount++
	prefix := strings.Repeat(">", callCount)

	lastMsg := ""
	defer func() {
		statusMsg(lastMsg, "")
	}()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		currLine++
		lastMsg = statusMsg(lastMsg, "%s %s %d: ", prefix, currFile, currLine)

		line := scanner.Text()
		s := strings.TrimLeft(line, " ")
		if s == "" || s == "\\" || strings.HasPrefix(s, "\\ ") {
			continue // don't send empty or comment-only lines
		}

		if strings.HasPrefix(line, "include ") {
			for _, fname := range strings.Split(line[8:], " ") {
				statusMsg(lastMsg, "")
				if !IncludeFile(fname, level+1) {
					return false
				}
			}
		} else {
			serialSend <- []byte(line + "\r")
			if !match(line) {
				return false
			}
		}
	}

	return true
}

// statusMsg prints a formatted string and returns it. It takes the previous
// string to be able to clear it before outputting the new message.
func statusMsg(prev string, desc string, args ...interface{}) string {
	msg := fmt.Sprintf(desc, args...)
	n := len(msg)
	// FIXME this optimisation is incorrect, it sometimes eats up first 3 chars
	if false && n > 3 && n == len(prev) && msg[:n-3] == prev[:n-3] {
		fmt.Print("\b\b\b", msg[n-3:]) // optimise if only end changes
	} else {
		if len(msg) < len(prev) {
			fmt.Print("\r", strings.Repeat(" ", len(prev)))
		}
		fmt.Print("\r", msg)
	}
	return msg
}

func match(expect string) bool {
	timer := time.NewTimer(3 * time.Second)

	var pending []byte
	for {
		select {

		case data := <-serialRecv:
			pending = append(pending, data...)
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(time.Second)

		case <-time.After(10 * time.Millisecond):
			if !bytes.Contains(pending, []byte{'\n'}) {
				continue
			}

			lines := bytes.Split(pending, []byte{'\n'})
			n := len(lines)
			for i := 0; i < n-2; i++ {
				fmt.Printf("%s\n", lines[i])
			}
			lines = lines[n-2:]

			last := string(lines[0])
			if len(lines[1]) == 0 {
				hasExpected := strings.HasPrefix(last, expect+" ")
				if hasExpected || strings.HasSuffix(last, " ok.") {
					if last != expect+"  ok." {
						msg := last
						// only show output if source does not end with ")"
						// in that case, show just the comment from "(" on
						if hasExpected {
							msg = last[len(expect)+1:]
							if strings.HasSuffix(expect, ")") {
								if n := strings.LastIndex(expect, "("); n > 0 {
									msg = last[n:]
								}
							}
						}
						fmt.Printf("%s\n", msg)
						if strings.HasSuffix(last, " not found.") ||
							strings.HasSuffix(last, " Stack underflow") ||
							strings.HasSuffix(last, " Jump too far") {
							return false // no point in keeping going
						}
					}
					return true
				}
			} else {
				fmt.Printf("TAIL? %#v\n", lines[1])
			}
			fmt.Printf("%s\n", last)
			pending = lines[1]

		case <-timer.C:
			if len(pending) == 0 {
				return true
			}
			fmt.Printf("%s (timeout)\n", pending)
			return string(pending) == expect+" "
		}
	}
}
