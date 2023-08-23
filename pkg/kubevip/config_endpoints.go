package kubevip

import (
	"fmt"
	"net/url"
	"strconv"

	log "github.com/sirupsen/logrus"
)

func init() {
	// Start the index negative as it will be incrememnted of first approach
	endPointIndex = -1
}

// ValidateBackEndURLS will run through the endpoints and ensure that they're a valid URL
func ValidateBackEndURLS(endpoints *[]BackEnd) error {
	if len(*endpoints) == 0 {
		return fmt.Errorf("No Backends configured")
	}

	for i := range *endpoints {
		log.Debugf("Parsing [%s]", (*endpoints)[i].RawURL)
		u, err := url.Parse((*endpoints)[i].RawURL)
		if err != nil {
			return err
		}

		// No error is returned if the prefix/schema is missing
		// If the Host is empty then we were unable to parse correctly (could be prefix is missing)
		if u.Host == "" {
			return fmt.Errorf("Unable to parse [%s], ensure it's prefixed with http(s)://", (*endpoints)[i].RawURL)
		}
		(*endpoints)[i].Address = u.Hostname()
		// if a port is specified then update the internal endpoint stuct, if not rely on the schema
		if u.Port() != "" {
			portNum, err := strconv.Atoi(u.Port())
			if err != nil {
				return err
			}
			(*endpoints)[i].Port = portNum
		}
		(*endpoints)[i].ParsedURL = u
	}
	return nil
}

// ReturnEndpointAddr - returns an endpoint
func (lb LoadBalancer) ReturnEndpointAddr() (*BackEnd, string, error) {
	if len(lb.Backends) == 0 {
		return nil, "", fmt.Errorf("No Backends configured")
	}
	if endPointIndex < len(lb.Backends)-1 {
		endPointIndex++
	} else {
		// reset the index to the beginning
		endPointIndex = 0
	}
	// TODO - weighting, decision algorythmn
	if lb.Backends[endPointIndex].IsAlive() {
		endpoint := fmt.Sprintf("%s:%d", lb.Backends[endPointIndex].Address, lb.Backends[endPointIndex].Port)
		log.Debugf("[%s] return endpoint [%s]", lb.Name, endpoint)
		return &lb.Backends[endPointIndex], endpoint, nil
	} else {
		allDown := true
		for x := range lb.Backends {
			if lb.Backends[x].IsAlive() {
				allDown = false
				break
			}
		}
		if allDown {
			errMsg := fmt.Sprintf("[%s] have no alive backend, refresh alive with true for all backend", lb.Name)
			log.Debugf(errMsg)
			for x := range lb.Backends {
				lb.Backends[x].SetAlive(&lb, true)
			}
			return lb.ReturnEndpointAddr()
		}
	}
	return lb.ReturnEndpointAddr()
}

// ReturnEndpointURL - returns an endpoint
func (lb LoadBalancer) ReturnEndpointURL() (*BackEnd, string, *url.URL, error) {
	if len(lb.Backends) == 0 {
		return nil, "", nil, fmt.Errorf("No Backends configured")
	}
	if endPointIndex != len(lb.Backends)-1 {
		endPointIndex++
	} else {
		// reset the index to the beginning
		endPointIndex = 0
	}
	// TODO - weighting, decision algorythmn
	if lb.Backends[endPointIndex].IsAlive() {
		endpoint := fmt.Sprintf("%s:%d", lb.Backends[endPointIndex].Address, lb.Backends[endPointIndex].Port)
		log.Debugf("[%s] return endpoint [%s]", lb.Name, endpoint)
		return &lb.Backends[endPointIndex], endpoint, lb.Backends[endPointIndex].ParsedURL, nil
	} else  {
		allDown := true
		for x := range lb.Backends {
			if lb.Backends[x].IsAlive() {
				allDown = false
				break
			}
		}
		if allDown {
			errMsg := fmt.Sprintf("[%s] have no alive backend, refresh alive with true for all backend", lb.Name)
			log.Debugf(errMsg)
			for x := range lb.Backends {
				lb.Backends[x].SetAlive(&lb, true)
			}
			return lb.ReturnEndpointURL()
		}
	}
	return lb.ReturnEndpointURL()
}

// SetAlive - set backend alive
func (b *BackEnd) SetAlive(lb *LoadBalancer, alive bool) {
	fullAddress := fmt.Sprintf("%s:%d", b.Address, b.Port)
	b.mux.Lock()
	b.Alive = alive
	if alive {
		log.Debugf("[%s] backend [%s] status [up]", lb.Name, fullAddress)
	} else {
		log.Warnf("[%s] backend [%s] status [down]", lb.Name, fullAddress)
	}
	b.mux.Unlock()
}

// IsAlive - return backend alive
func (b *BackEnd) IsAlive() (alive bool) {
	b.mux.RLock()
	alive = b.Alive
	b.mux.RUnlock()
	return
}
