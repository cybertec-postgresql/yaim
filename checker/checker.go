package checker

import (
	"errors"

	"github.com/cybertec-postgresql/yaim/config"
)

// ErrUnsupportedEndpointType is returned for an unsupported endpoint
var ErrUnsupportedCheckerType = errors.New("given endpoint type not supported")

// LeaderChecker is the interface for checking leadership
type Checker interface {
	Check() error
	CompareExpected() bool
	IsHealthy() (bool, error)
}

// NewLeaderChecker returns a new LeaderChecker instance depending on the configuration
func NewChecker(conf *config.Config) (Checker, error) {
	var c Checker
	var err error

	switch conf.CheckerType {
	// case "postgres":
	// 	c, err = NewPostgresChecker(con)
	// case "shell":
	// 	c, err = NewShellChecker(con)
	case "http":
		c, err = NewHttpChecker(conf)
	default:
		err = ErrUnsupportedCheckerType
	}

	return c, err
}

// func (c *Checker) checkLoop(ctx context.Context, out chan<- bool) error {
// 	for {
// 		if ctx.Err() != nil {
// 			break
// 		}

// 		check, checkErr := c.check()
// 		if checkErr != nil {
// 			log.Printf("DCS error: %s", checkErr)
// 			out <- false
// 			time.Sleep(time.Duration(eConf.Interval) * time.Millisecond)
// 			continue
// 		}

// 		comp, compErr := c.compareExpected()
// 		if compErr != nil {
// 			log.Printf("compare error: %s", compErr)
// 			out <- false
// 			continue
// 		}

// 		select {
// 		case <-ctx.Done():
// 			break
// 		case out <- comp:
// 			time.Sleep(time.Duration(eConf.Interval) * time.Millisecond)
// 			continue
// 		}
// 	}

// 	return ctx.Err()
// }
