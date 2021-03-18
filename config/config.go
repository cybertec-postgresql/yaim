package config

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var versionString = "0.0.1"

// Config represents the configuration of the VIP manager
type Config struct {
	Mask  int    `mapstructure:"netmask"`
	Iface string `mapstructure:"interface"`

	HostingType string `mapstructure:"manager-type"`

	Nodename string `mapstructure:"nodename"` //hostname to trigger on. usually the name of the host where this vip-manager runs.

	DcsType      string   `mapstructure:"dcs-type"`
	DcsEndpoints []string `mapstructure:"dcs-endpoints"`

	DcsNamespace string `mapstructure:"dcs-namespace"`

	CheckerType string `mapstructure:"checker-type"`

	HttpUrl                      string `mapstructure:"http-url"`
	HttpUser                     string `mapstructure:"http-user"`
	HttpPassword                 string `mapstructure:"http-password"`
	HttpCAFile                   string `mapstructure:"http-ca-file"`
	HttpCertFile                 string `mapstructure:"http-cert-file"`
	HttpKeyFile                  string `mapstructure:"http-key-file"`
	HttpExpectedCode             int    `mapstructure:"http-expected-code"`
	HttpExpectedResponse         string `mapstructure:"http-expected-response"`
	HttpExpectedResponseContains string `mapstructure:"http-expected-response-contains"`

	PostgresConnUrl          string `mapstructure:"postgres-conn-url"`
	PostgresQuery            string `mapstructure:"postgres-query"`
	PostgresUser             string `mapstructure:"postgres-user"`
	PostgresPassword         string `mapstructure:"postgres-password"`
	PostgresCAFile           string `mapstructure:"postgres-ca-file"`
	PostgresCertFile         string `mapstructure:"postgres-cert-file"`
	PostgresKeyFile          string `mapstructure:"postgres-key-file"`
	PostgresExpectedResponse string `mapstructure:"postgres-exprected-response"`

	EtcdUser     string `mapstructure:"etcd-user"`
	EtcdPassword string `mapstructure:"etcd-password"`
	EtcdCAFile   string `mapstructure:"etcd-ca-file"`
	EtcdCertFile string `mapstructure:"etcd-cert-file"`
	EtcdKeyFile  string `mapstructure:"etcd-key-file"`

	ConsulToken string `mapstructure:"consul-token"`

	TTL int `mapstructure:"ttl"`

	Interval int `mapstructure:"interval"` //milliseconds

	RetryAfter int `mapstructure:"retry-after"` //milliseconds
	RetryNum   int `mapstructure:"retry-num"`

	LogLevel string `mapstructure:"log-level"` // Trace, Debug, Info, Warning, Error, Fatal and Panic
}

func defineFlags() {
	// When adding new flags here, consider adding them to the Config struct above
	// and then make sure to insert them into the conf instance in NewConfig down below.
	pflag.String("config", "", "Location of the configuration file.")
	pflag.Bool("version", false, "Show the version number.")
	pflag.CommandLine.SortFlags = false
}

func setDefaults() {
	defaults := map[string]string{
		"dcs-type":    "etcd",
		"interval":    "1000",
		"hostingtype": "basic",
		"retry-num":   "3",
		"retry-after": "250",
		"log-level":   "Info",
	}

	for k, v := range defaults {
		if !viper.IsSet(k) {
			viper.SetDefault(k, v)
		}
	}
}

func checkSetting(name string) bool {
	if !viper.IsSet(name) {
		log.Printf("Setting %s is mandatory", name)
		return false
	}
	return true
}

func checkMandatory() error {
	mandatory := []string{
		"netmask",
		"interface",
		"nodename",
		"dcs-endpoints",
	}
	success := true
	for _, v := range mandatory {
		success = checkSetting(v) && success
	}
	if !success {
		return errors.New("one or more mandatory settings were not set")
	}
	return nil
}

// if reason is set, but implied is not set, return false.
func checkImpliedSetting(implied string, reason string) bool {
	if viper.IsSet(reason) && !viper.IsSet(implied) {
		log.Printf("Setting %s is mandatory when setting %s is specified.", implied, reason)
		return false
	}
	return true
}

// Some settings imply that another setting must be set as well.
func checkImpliedMandatory() error {
	mandatory := map[string]string{
		// "implied" : "reason"
		"etcd-user":     "etcd-password",
		"etcd-key-file": "etcd-cert-file",
		"etcd-ca-file":  "etcd-cert-file",
	}
	success := true
	for k, v := range mandatory {
		success = checkImpliedSetting(k, v) && success
	}
	if !success {
		return errors.New("one or more implied mandatory settings were not set")
	}
	return nil
}

func printSettings() {
	s := []string{}

	for k, v := range viper.AllSettings() {
		if v != "" {
			switch k {
			case "etcd-password":
				fallthrough
			case "consul-token":
				s = append(s, fmt.Sprintf("\t%s : *****\n", k))
			default:
				s = append(s, fmt.Sprintf("\t%s : %v\n", k, v))
			}
		}
	}

	sort.Strings(s)
	log.Println("This is the config that will be used:")
	for k := range s {
		fmt.Print(s[k])
	}
}

// NewConfig returns a new Config instance
func NewConfig() (*Config, error) {
	var err error

	defineFlags()
	pflag.Parse()
	// import pflags into viper
	_ = viper.BindPFlags(pflag.CommandLine)

	// make viper look for env variables that are prefixed VIP_...
	// e.g.: viper.getString("ip") will return the value of env variable VIP_IP
	viper.SetEnvPrefix("yaim")
	viper.AutomaticEnv()
	//replace dashes (in flags) with underscores (in ENV vars)
	// so that e.g. viper.GetString("dcs-endpoints") will return value of VIP_DCS_ENDPOINTS
	replacer := strings.NewReplacer("-", "_")
	viper.SetEnvKeyReplacer(replacer)

	// viper precedence order
	// - explicit call to Set
	// - flag
	// - env
	// - config
	// - key/value store
	// - default

	// if a configfile has been passed, make viper read it
	if viper.IsSet("config") {
		viper.SetConfigFile(viper.GetString("config"))

		err := viper.ReadInConfig() // Find and read the config file
		if err != nil {             // Handle errors reading the config file
			return nil, fmt.Errorf("Fatal error reading config file: %w", err)
		}
		log.Printf("Using config from file: %s\n", viper.ConfigFileUsed())
	}

	setDefaults()

	logLevel, err := log.ParseLevel(viper.GetString("log-level"))
	if err == nil {
		log.SetLevel(logLevel)
	}

	// convert string of csv to String Slice
	if viper.IsSet("dcs-endpoints") {
		endpointsString := viper.GetString("dcs-endpoints")
		if strings.Contains(endpointsString, ",") {
			viper.Set("dcs-endpoints", strings.Split(endpointsString, ","))
		}
	}

	// apply defaults for endpoints
	if !viper.IsSet("dcs-endpoints") {
		log.Println("No dcs-endpoints specified, trying to use localhost with standard ports!")

		switch viper.GetString("dcs-type") {
		case "consul":
			viper.Set("dcs-endpoints", []string{"http://127.0.0.1:8500"})
		case "etcd":
			viper.Set("dcs-endpoints", []string{"http://127.0.0.1:2379"})
		}
	}

	// set trigger-value to hostname if nothing is specified
	if len(viper.GetString("trigger-value")) == 0 {
		nodename, err := os.Hostname()
		if err != nil {
			log.Printf("No nodename specified, hostname could not be retrieved: %s", err)
		} else {
			log.Printf("No nodename specified, instead using hostname: %v", nodename)
			viper.Set("nodename", nodename)
		}
	}

	if err = checkMandatory(); err != nil {
		return nil, err
	}

	if err = checkImpliedMandatory(); err != nil {
		return nil, err
	}

	conf := &Config{}
	err = viper.Unmarshal(conf)
	if err != nil {
		log.Fatalf("unable to decode viper config into config struct, %v", err)
	}

	printSettings()

	return conf, nil
}
