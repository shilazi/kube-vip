package kubevip

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
)

var endPointIndex int // Holds the previous endpoint (for determining decisions on next endpoint)

//ParseBackendConfig -
func ParseBackendConfig(ep string) (*BackEnd, error) {
	endpoint := strings.Split(ep, ":")
	if len(endpoint) != 2 {
		return nil, fmt.Errorf("Ensure a backend is in in the format address:port, e.g. 10.0.0.1:8080")
	}
	p, err := strconv.Atoi(endpoint[1])
	if err != nil {
		return nil, err
	}
	return &BackEnd{Address: endpoint[0], Port: p}, nil
}

//ParsePeerConfig -
func ParsePeerConfig(ep string) (*RaftPeer, error) {
	endpoint := strings.Split(ep, ":")
	if len(endpoint) != 3 {
		return nil, fmt.Errorf("Ensure a peer is in in the format id:address:port, e.g. server1:10.0.0.1:8080")
	}
	p, err := strconv.Atoi(endpoint[2])
	if err != nil {
		return nil, err
	}
	return &RaftPeer{ID: endpoint[0], Address: endpoint[1], Port: p}, nil
}

//OpenConfig will attempt to read a file and parse it's contents into a configuration
func OpenConfig(path string) (*Config, error) {
	if path == "" {
		return nil, fmt.Errorf("Path cannot be blank")
	}

	log.Infof("Reading configuration from [%s]", path)

	// Check the actual path from the string
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		// Attempt to read the data
		configData, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, err
		}

		// If data is read succesfully parse the yaml
		var c Config
		err = yaml.Unmarshal(configData, &c)
		if err != nil {
			return nil, err
		}
		return &c, nil

	}
	return nil, fmt.Errorf("Error reading [%s]", path)
}

//PrintConfig - will print out an instance of the kubevip config
func (c *Config) PrintConfig() {
	b, _ := yaml.Marshal(c)

	fmt.Printf(string(b))
}

//ParseFlags will write the current configuration to a specified [path]
func (c *Config) ParseFlags(localPeer string, remotePeers, backends []string) error {
	// Parse localPeer
	p, err := ParsePeerConfig(localPeer)
	if err != nil {
		return err
	}
	c.LocalPeer = *p

	// Parse remotePeers
	//Iterate backends
	for i := range remotePeers {
		p, err := ParsePeerConfig(remotePeers[i])
		if err != nil {
			return err

		}
		c.RemotePeers = append(c.RemotePeers, *p)
	}

	//Iterate backends
	for i := range backends {
		b, err := ParseBackendConfig(backends[i])
		if err != nil {
			return err
		}
		c.LoadBalancers[0].Backends = append(c.LoadBalancers[0].Backends, *b)
	}

	return nil
}

//SampleConfig will create an example configuration and write it to the specified [path]
func SampleConfig() {

	// Generate Sample configuration
	c := &Config{
		// Generate sample peers
		RemotePeers: []RaftPeer{
			{
				ID:      "server2",
				Address: "192.168.0.2",
				Port:    10000,
			},
			{
				ID:      "server3",
				Address: "192.168.0.3",
				Port:    10000,
			},
		},
		LocalPeer: RaftPeer{
			ID:      "server1",
			Address: "192.168.0.1",
			Port:    10000,
		},
		// Virtual IP address
		VIP: "192.168.0.100",
		// Interface to bind to
		Interface: "eth0",
		// Load Balancer Configuration
		LoadBalancers: []LoadBalancer{
			{
				Name:      "Kubernetes Control Plane",
				Type:      "tcp",
				Port:      6444,
				BindToVip: true,
				EnableProxyProtocol: false,
				Backends: []BackEnd{
					{
						Address: "192.168.0.100",
						Port:    6443,
					},
					{
						Address: "192.168.0.101",
						Port:    6443,
					},
					{
						Address: "192.168.0.102",
						Port:    6443,
					},
				},
			},
		},
	}
	b, _ := yaml.Marshal(c)

	fmt.Printf(string(b))
}

//WriteConfig will write the current configuration to a specified [path]
func (c *Config) WriteConfig(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	bytesWritten, err := f.Write(b)
	if err != nil {
		return err
	}
	log.Debugf("wrote %d bytes\n", bytesWritten)
	return nil
}
