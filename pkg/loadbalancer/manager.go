package loadbalancer

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/plunder-app/kube-vip/pkg/kubevip"
	log "github.com/sirupsen/logrus"
)

const (
	// reset alive timer period, every 30 second
	resetAlivePeriod = time.Second * 30
	// net.Dial Timeout, default 0.5 sec
	dialTMOUT = time.Millisecond * 500
)

//LBInstance - manages the state of load balancer instances
type LBInstance struct {
	stop     chan bool             // Asks LB to stop
	stopped  chan bool             // LB is stopped
	instance *kubevip.LoadBalancer // pointer to a LB instance
	//	mux      sync.Mutex
	backendIndex *int              // The backend index for LB instance
}

//LBManager - will manage a number of load blancer instances
type LBManager struct {
	loadBalancer []LBInstance
}

//Add - handles the building of the load balancers
func (lm *LBManager) Add(bindAddress string, lb *kubevip.LoadBalancer) error {
	// Start the index negative as it will be incrememnted of first approach
	initBackendIndex := -1
	newLB := LBInstance{
		stop:     make(chan bool, 1),
		stopped:  make(chan bool, 1),
		instance: lb,
		backendIndex: &initBackendIndex,
	}

	network := strings.ToLower(lb.Type)
	switch network {
	case "tcp":
		err := newLB.startTCP(bindAddress)
		if err != nil {
			return err
		}
	case "udp":
		err := newLB.startUDP(bindAddress)
		if err != nil {
			return err
		}
	case "http":
		err := newLB.startHTTP(bindAddress)
		if err != nil {
			return err
		}
		// set to 'tcp' for dial
		network = "tcp"
	default:
		return fmt.Errorf("Unknown Load Balancer type [%s]", lb.Type)
	}

	// start backend reset alive timer
	go func(l *LBInstance, network string) {
		log.Infof("Staring load Balancer [%s] backend reset alive timer", l.instance.Name)

		t := time.NewTicker(resetAlivePeriod)

		defer func() {
			t.Stop()
			log.Infof("Load Balancer [%s] backend reset alive timer has stopped", l.instance.Name)
		}()

		for {
			select {
			case <-l.stop:
				return
			case <-t.C:
				for x := range l.instance.Backends {
					if !l.instance.Backends[x].IsAlive() {
						go func(backends []kubevip.BackEnd, y int) {
							fullAddress := fmt.Sprintf("%s:%d", backends[y].Address, backends[y].Port)
							conn, err := net.DialTimeout(network, fullAddress, dialTMOUT)
							if err != nil {
								log.Warnf("unreachable, error: %v", err)
							} else {
								l.instance.Backends[x].SetAlive(l.instance, true)
								conn.Close()
							}
						}(l.instance.Backends, x)
					}
				}
			}
		}
	}(&newLB, network)

	lm.loadBalancer = append(lm.loadBalancer, newLB)
	return nil
}

//StopAll - handles the building of the load balancers
func (lm *LBManager) StopAll() error {
	log.Debugf("Stopping [%d] loadbalancer instances", len(lm.loadBalancer))
	for x := range lm.loadBalancer {
		err := lm.loadBalancer[x].Stop()
		if err != nil {
			return err
		}
	}
	// Reset the loadbalancer entries
	lm.loadBalancer = nil
	return nil
}

//Stop - handles the building of the load balancers
func (l *LBInstance) Stop() error {

	close(l.stop)

	<-l.stopped
	log.Infof("Load Balancer instance [%s] has stopped", l.instance.Name)
	return nil
}
