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
			if !ok {
				close(ch)
				return
			}
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

func waitFor(port int, state bool, debounce time.Duration) (<-chan bool, func(), error) {
	pi := gopisysfs.GetPi()

	var ondie func()
	defer func() {
		if ondie != nil {
			ondie()
		}
	}()

	var err error

	gpio, err := pi.GetPort(port)
	if err != nil {
		return nil, nil, err
	}

	err = gpio.Enable()
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		gpio.Reset()
	}

	ondie = cleanup

	err = gpio.SetMode(gopisysfs.GPIOInput)
	if err != nil {
		return nil, nil, err
	}

	inputs, err := gpio.Values(100)
	if err != nil {
		return nil, nil, err
	}

	ch := make(chan bool, 1)

	go monitorPort(inputs, state, debounce, ch)

	ondie = nil

	return ch, cleanup, nil

}

func runflash(gpio gopisysfs.GPIOPort, ch <-chan bool) {
	val := false
	for {
		_, ok := <-ch
		if !ok {
			return
		}
		val = !val
		gpio.SetValue(val)
	}
}

func flash(port int) (chan<- bool, func(), error) {
	pi := gopisysfs.GetPi()

	var ondie func()
	defer func() {
		if ondie != nil {
			ondie()
		}
	}()

	var err error

	gpio, err := pi.GetPort(port)
	if err != nil {
		return nil, nil, err
	}

	err = gpio.Enable()
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		gpio.Reset()
	}
	ondie = cleanup

	err = gpio.SetMode(gopisysfs.GPIOOutputLow)
	if err != nil {
		return nil, nil, err
	}

	err = gpio.SetValue(false)
	if err != nil {
		return nil, nil, err
	}

	ch := make(chan bool, 100)

	go runflash(gpio, ch)

	ondie = nil

	return ch, cleanup, nil

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
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
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

	command := flag.Args()
	if len(command) == 0 {
		return fmt.Errorf("Command to run has not been specified - need arguments to %v", me)
	}

	log.Printf("OS Args: %v\n", os.Args)
	log.Printf("Running %v monitoring port %v for state %v with LED on %v and command: '%v'\n", me, btnport, waitfor, ledport, strings.Join(command, "', '"))

	flasher, fclean, err := flash(ledport)
	if err != nil {
		return fmt.Errorf("Unable to activate LED on port %v: %v", ledport, err)
	}
	defer fclean()

	event, bclean, err := waitFor(btnport, !dilow, debounce)
	if err != nil {
		return fmt.Errorf("Unable to activate listener on port %v: %v", btnport, err)
	}
	defer bclean()

	// catch various kill signals.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			flasher <- true
		case <-event:
			err := runCommand(flasher, command)
			if err != nil {
				return err
			}
		case s := <-sigc:
			return fmt.Errorf("Signal received: %v\n", s)
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
