package machine

import (
	log "github.com/sirupsen/logrus"
	"periph.io/x/periph/conn/gpio"
	"periph.io/x/periph/conn/gpio/gpioreg"
	"periph.io/x/periph/host"
	"sync"
	"time"
)

const (
	touchPin  = "4"  // BCM 4 = #7
	motorPin  = "27" // BCM 27 = #13
	buzzerPin = "17" // BCM 17 = #11
)

type DispenserMachine struct {
	touchEvents  chan bool      // Internal sending channel for touch events
	motorEvents  chan bool      // Internal motor events channel
	buzzerEvents chan bool      // Internal buzzer events channel
	done         chan bool      // Internal done channel
	waitGroup    sync.WaitGroup // Internal goroutine WaitGroup
}

func NewDispenserMachine() *DispenserMachine {
	touchEvents := make(chan bool)
	motorEvents := make(chan bool)
	buzzerEvents := make(chan bool)

	m := &DispenserMachine{
		touchEvents:  touchEvents,
		motorEvents:  motorEvents,
		buzzerEvents: buzzerEvents,
	}

	return m
}

func (m *DispenserMachine) Start() error {
	if _, err := host.Init(); err != nil {
		return err
	}

	m.done = make(chan bool)

	var waitGroup sync.WaitGroup
	m.waitGroup = waitGroup

	go m.handleTouch()
	go m.driveMotor()
	go m.driveBuzzer()

	return nil
}

func (m *DispenserMachine) Stop() {
	log.Info("Stopping machine...")

	close(m.done)

	// Blocking until all goroutines finished executing
	m.waitGroup.Wait()

	log.Info("Machine stopped")
}

func (m *DispenserMachine) ToggleMotor(on bool) {
	log.Infof("Toggling motor %v", on)
	m.motorEvents <- on
}

func (m *DispenserMachine) ToggleBuzzer(on bool) {
	log.Infof("Toggling buzzer %v", on)
	m.buzzerEvents <- on
}

func (m *DispenserMachine) TouchEvents() <-chan bool {
	return m.touchEvents
}

func (m *DispenserMachine) handleTouch() {
	log.Info("Starting to handle touch events")

	m.waitGroup.Add(1)
	defer m.waitGroup.Done()

	p := gpioreg.ByName(touchPin)

	// set as input, with an internal pull down resistor
	if err := p.In(gpio.PullDown, gpio.BothEdges); err != nil {
		log.Fatal(err)
	}

	// Turn blocking WaitForEdge() func into channel
	edges := make(chan bool)
	go func() {
		// m.waitGroup.Add(1)
		// defer m.waitGroup.Done()

		// TODO: Stop this goroutine on done signal

		var notifyAfterThrottledTime = time.Time{}
		var hasSentHigh = false

		for {
			p.WaitForEdge(-1)

			if p.Read() == gpio.High {
				if notifyAfterThrottledTime.IsZero() {
					// just save time for throttling
					notifyAfterThrottledTime = time.Now().Add(2 * time.Millisecond)
				} else if !hasSentHigh && time.Now().After(notifyAfterThrottledTime) {
					// send throttled touch start
					edges <- true

					// make sure next high signal isn't sent anymore
					hasSentHigh = true
				}
			} else if hasSentHigh {
				// reset time for throttling
				notifyAfterThrottledTime = time.Time{}

				// send touch stop
				edges <- false

				// make sure the next high is sent
				hasSentHigh = false
			} else {
				// reset time for throttling
				notifyAfterThrottledTime = time.Time{}
			}
		}
	}()

	for {
		select {
		case touch := <-edges:
			log.WithField("pin", "touch").WithField("on", touch).Info("Received touch event")
			m.touchEvents <- touch
		case <-m.done:
			log.Info("Got done event in handleTouch")

			return
		}
	}

	log.Debug("Leaving handleTouch goroutine")
}

func (m *DispenserMachine) driveMotor() {
	log.Info("Starting to handle motor events")

	m.waitGroup.Add(1)
	defer m.waitGroup.Done()

	p := gpioreg.ByName(motorPin)

	for {
		select {
		case on := <-m.motorEvents:
			log.WithField("pin", "motor").WithField("on", on).Info("Received motor event")

			if on {
				p.Out(gpio.High)
			} else {
				p.Out(gpio.Low)
			}
		case <-m.done:
			log.Info("Got done event in driveMotor")

			p.Out(gpio.Low)
			return
		}
	}

	log.Debug("Leaving driveMotor goroutine")
}

func (m *DispenserMachine) driveBuzzer() {
	log.Info("Starting to handle buzzer events")

	m.waitGroup.Add(1)
	defer m.waitGroup.Done()

	p := gpioreg.ByName(buzzerPin)

	for {
		select {
		case on := <-m.buzzerEvents:
			log.WithField("pin", "buzzer").WithField("on", on).Info("Received buzzer event")

			if on {
				p.Out(gpio.High)
			} else {
				p.Out(gpio.Low)
			}
		case <-m.done:
			log.Info("Got done event in driveBuzzer")

			p.Out(gpio.Low)
			return
		}
	}

	log.Debug("Leaving driveBuzzer goroutine")
}

func (m *DispenserMachine) DiagnosticNoise() {
	m.ToggleBuzzer(true)
	time.Sleep(200 * time.Millisecond)
	m.ToggleBuzzer(false)
	time.Sleep(200 * time.Millisecond)
	m.ToggleBuzzer(true)
	time.Sleep(200 * time.Millisecond)
	m.ToggleBuzzer(false)
}
