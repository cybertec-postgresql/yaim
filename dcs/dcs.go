package dcs

import (
	"errors"

	"github.com/cybertec-postgresql/yaim/config"
)

// ErrUnsupportedEndpointType is returned for an unsupported endpoint
var ErrUnsupporteDCSType = errors.New("given endpoint type not supported")

// LeaderChecker is the interface for checking leadership
type Dcs interface {
	AdvertiseInDCS()
	MarkIpInDCS(ip string) (success bool)
	RefreshMarkIpInDCS(ip string)
	UnMarkIpInDCS(ip string)
	GetNumberAdvertisments() (num int, err error)
	GetNumberMarkedIPs() (numIps int, ownMarkedIPs, unmarkedIPs []string, err error)
}

// NewLeaderChecker returns a new LeaderChecker instance depending on the configuration
func NewDcs(conf *config.Config) (Dcs, error) {
	var d Dcs
	var err error

	switch conf.DcsType {
	// case "postgres":
	// 	c, err = NewPostgresChecker(con)
	// case "shell":
	// 	c, err = NewShellChecker(con)
	case "etcd":
		d, err = NewEtcdDcs(conf)
	default:
		err = ErrUnsupporteDCSType
	}

	return d, err
}
