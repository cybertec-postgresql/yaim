package checker

import (
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/cybertec-postgresql/yaim/config"
	log "github.com/sirupsen/logrus"
)

type HttpChecker struct {
	conf   *config.Config
	code   int
	result string
}

func NewHttpChecker(conf *config.Config) (*HttpChecker, error) {
	var c = new(HttpChecker)
	c.conf = conf

	return c, nil
}

func (c *HttpChecker) Check() error {
	resp, err := http.Get(c.conf.HttpUrl)
	if err != nil {
		// handle error
		return err
	}
	defer resp.Body.Close()

	c.code = resp.StatusCode

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// handle error
		return err
	}

	c.result = string(body)

	log.Debug("The http health check query returned: ", c.result)

	return nil
}

func (c *HttpChecker) CompareExpected() bool {
	//Code needs to match expectation
	//if there is an expectation for the result, that needs to match as well
	if c.code == c.conf.HttpExpectedCode {
		if c.conf.HttpExpectedResponse != "" {
			return c.result == c.conf.HttpExpectedResponse
		}
		if c.conf.HttpExpectedResponseContains != "" {
			return strings.Contains(c.result, c.conf.HttpExpectedResponseContains)
		}
		return true
	}
	return false
	//#return c.code == c.conf.HttpExpectedCode && (c.conf.HttpExpectedResponse == "" || c.result == c.conf.HttpExpectedResponse)
}

func (c *HttpChecker) IsHealthy() (bool, error) {
	err := c.Check()
	if err != nil {
		return false, err
	}
	return c.CompareExpected(), nil
}
