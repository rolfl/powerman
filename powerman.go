package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rolfl/gopisysfs"
)

func monitorPort(vals <-chan gopisysfs.Event, state bool, debounce time.Duration, ch chan<- bool) {
	var bounceout <-chan time.Time
	bounceout = nil
	for {
		select {
		case e, ok := <-vals:
			if e.Value == state {
				bounceout = time.After(debounce)
			} else {
				bounceout = nil
			}
		case <-bounceout:
			// debounce success
			bounceout = nil
			ch <- true
		}
	}
}

func waitFor(port int, state bool, debounce time.Duration) (<-chan bool, error) {
	pi := gopisysfs.GetPi()

	var err error

	gpio, err := pi.GetPort(port)
	if err != nil {
		return nil, err
	}

	err = gpio.Enable()
	if err != nil {
		return nil, err
	}

	err = gpio.SetMode(gopisysfs.GPIOInput)
	if err != nil {
		return nil, err
	}

	inputs, err := gpio.Values(100)

	ch := make(chan bool, 1)

	go monitorPort(inputs, state, debounce, ch)

	return ch, nil

}

func runflash(gpio gopisysfs.GPIOPort, ch <-chan bool) {
	val := false
	for {
		_, ok := <-ch
		if !ok {
			return
		}
		val = !val
		gpio.SetVal(val)
	}
}

func flash(int port) (chan<- bool, error) {
	pi := gopisysfs.GetPi()

	var err error

	gpio, err := pi.GetPort(port)
	if err != nil {
		return nil, err
	}

	err = gpio.Enable()
	if err != nil {
		return nil, err
	}

	err = gpio.SetMode(gopisysfs.GPIOOutputLow)
	if err != nil {
		return nil, err
	}

	err = gpio.SetValue(false)
	if err != nil {
		return nil, err
	}

	ch := make(chan bool, 100)

	go runflash(gpio, ch)

	return ch, nil

}

func runCommand(flasher chan<- bool, command []string) error {
	if len(command) == 0 {
		return fmt.Errorf("Invalid command: Cannot be empty")
	}

	flasher <- true
	dead := make(chan bool, 1)
	defer close(dead)

	go func() {
		tick := time.NewTicker(200 * time.Millisecond)
		for {
			select {
			case <-tick:
				flasher <- true
			case <-dead:
				return
			}
		}
	}()

	cmd := exec.Command(command[0], command[1:]...)
	co, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Error running command '%v'. Got error: '%v' and output:\n%v\n", strings.Join(command, "' '"), err, string(co))
	}
	return nil
}

func run() error {

	var dilow = true
	var ledport = 19
	var btnport = 26
	var debounce = 5 * time.Second
	var command = []string{"shutdown", "-h", "now"}

	command = []string{"sleep", "5"}

	var me = filepath.Base(os.Args[0])

	flag.BoolVar(&dilow, "activelow", dilow, "shutdown when signal goes low")
	flag.IntVar(&btnport, "button", btnport, "Port on which to listen for button events")
	flag.IntVar(&ledport, "led", ledport, "Port on which to display power state")
	flag.DurationVar(&debounce, "debounce", debounce, "Button hold time")
	flag.Parse()

	var waitfor = "high"
	if dilow {
		waitfor = "low"
	}

	if len(os.Args) > 1 {
		command = os.Args[1:]
	}

	log.Printf("OS Args: %v\n", os.Args)
	log.Printf("Running %v monitoring port %v for state %v with LED on %v and command: '%v'\n", me, btnport, waitfor, ledport, strings.Join(command, "', '"))

	flasher, err := flash(ledport)
	if err != nil {
		return fmt.Errorf("Unable to activate LED on port %v: %v", ledport, err)
	}

	event, err := waitFor(btnport, waitfor, debounce)
	if err != nil {
		return fmt.Errorf("Unable to activate listener on port %v: %v", btnport, err)
	}

	// catch various kill signals.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	tick := time.NewTicker(500 * time.Millisecond)
	for {
		select {
		case <-tick:
			flasher <- true
		case <-event:
			err := runcommand(flasher, command)
			if err != nil {
				return err
			}
		case s := <-sigc:
			return fmt.Errorln("Signal received: %v\n", s)
		}
	}
	return nil
}

func main() {
	err := run()
	if err != nil {
		log.Fatalf("Exiting: %v", err)
	}
}
