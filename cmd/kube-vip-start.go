package cmd

import (
	"fmt"
	"github.com/ghodss/yaml"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/plunder-app/kube-vip/pkg/cluster"
	"github.com/plunder-app/kube-vip/pkg/kubevip"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// Start as a single node (no cluster), start as a leader in the cluster
var startConfig kubevip.Config
var startConfigLB kubevip.LoadBalancer
var startLocalPeer, startKubeConfigPath string
var startRemotePeers, startBackends []string
var inCluster bool

func init() {
	// Get the configuration file
	kubeVipStart.Flags().StringVarP(&configPath, "config", "c", "", "Path to a kube-vip configuration")
	kubeVipStart.Flags().BoolVarP(&disableVIP, "disableVIP", "d", false, "Disable the VIP functionality")

	// Pointers so we can see if they're nil (and not called)
	kubeVipStart.Flags().StringVar(&startConfig.Interface, "interface", "eth0", "Name of the interface to bind to")
	kubeVipStart.Flags().StringVar(&startConfig.VIP, "vip", "192.168.0.1", "The Virtual IP address")
	kubeVipStart.Flags().BoolVar(&startConfig.SingleNode, "singleNode", false, "Start this instance as a single node")
	kubeVipStart.Flags().BoolVar(&startConfig.StartAsLeader, "startAsLeader", false, "Start this instance as the cluster leader")
	kubeVipStart.Flags().BoolVar(&startConfig.GratuitousARP, "arp", false, "Use ARP broadcasts to improve VIP re-allocations")
	kubeVipStart.Flags().StringVar(&startLocalPeer, "localPeer", "server1:192.168.0.1:10000", "Settings for this peer, format: id:address:port")
	kubeVipStart.Flags().StringSliceVar(&startRemotePeers, "remotePeers", []string{"server2:192.168.0.2:10000", "server3:192.168.0.3:10000"}, "Comma seperated remotePeers, format: id:address:port")
	// Load Balancer flags
	kubeVipStart.Flags().BoolVar(&startConfigLB.BindToVip, "lbBindToVip", false, "Bind example load balancer to VIP")
	kubeVipStart.Flags().StringVar(&startConfigLB.Type, "lbType", "tcp", "Type of load balancer instance (TCP/HTTP)")
	kubeVipStart.Flags().StringVar(&startConfigLB.Name, "lbName", "Example Load Balancer", "The name of a load balancer instance")
	kubeVipStart.Flags().IntVar(&startConfigLB.Port, "lbPort", 6444, "Port that load balancer will expose on")
	kubeVipStart.Flags().IntVar(&startConfigLB.BackendPort, "lbBackEndPort", 6443, "A port that all backends may be using (optional)")
	kubeVipStart.Flags().StringSliceVar(&startBackends, "lbBackends", []string{"192.168.0.1:6443", "192.168.0.2:6443"}, "Comma seperated backends, format: address:port")

	// Cluster configuration
	kubeVipStart.Flags().StringVar(&startKubeConfigPath, "kubeConfig", "/etc/kubernetes/admin.conf", "The path of a kubernetes configuration file")
	kubeVipStart.Flags().BoolVar(&inCluster, "inCluster", false, "Use the incluster token to authenticate to Kubernetes")
	kubeVipStart.Flags().BoolVar(&startConfig.EnableLeaderElection, "leaderElection", false, "Use the Kubernetes leader election mechanism for clustering")

}

var kubeVipStart = &cobra.Command{
	Use:   "start",
	Short: "Start the Virtual IP / Load balancer",
	Run: func(cmd *cobra.Command, args []string) {
		// Set the logging level for all subsequent functions
		log.SetLevel(log.Level(logLevel))
		var err error

		// If a configuration file is loaded, then it will overwrite flags

		if configPath != "" {
			c, err := kubevip.OpenConfig(configPath)
			if err != nil {
				log.Fatalf("%v", err)
			}
			startConfig = *c
		}

		// parse environment variables, these will overwrite anything loaded or flags
		err = kubevip.ParseEnvironment(&startConfig)
		if err != nil {
			log.Fatalln(err)
		}

		if log.GetLevel() >= log.DebugLevel {
			config, _ := yaml.Marshal(startConfig)
			// for log output orderly
			time.Sleep(time.Millisecond * 500)
			fmt.Println(string(config))
			time.Sleep(time.Millisecond * 500)
		}

		if startConfig.AddPeersAsBackends {
			log.Warnln("AddPeersAsBackends is true, will append raft peers as backends")
		}

		// http type of lb exist flag
		httpExistFlag := false

		// 1. add raft peers.address as backend.address
		// 2. set backend.port with backendPort
		for lx := range startConfig.LoadBalancers {
			lb := &startConfig.LoadBalancers[lx]

			// default port for backend, if not set alone
			var backendPort int
			if backendPort = lb.BackendPort; backendPort == 0 {
				backendPort = lb.Port
			}

			// if not set, make it
			if len(lb.Backends) == 0 {
				lb.Backends = make([]kubevip.BackEnd, 0)
			}

			// add raft peers address as backend address
			if startConfig.AddPeersAsBackends {
				lb.Backends = append(lb.Backends, kubevip.BackEnd{
					Address: startConfig.LocalPeer.Address,
				})

				for x := range startConfig.RemotePeers {
					lb.Backends = append(lb.Backends, kubevip.BackEnd{
						Address: startConfig.RemotePeers[x].Address,
					})
				}
			}

			// format rawURL then pick address, port or set blank
			if strings.ToLower(lb.Type) == "http" {
				// log and exit
				if httpExistFlag {
					log.Fatalln("Only one http-type load balancer is supported")
				}
				// set true
				httpExistFlag = true

				if lx != len(startConfig.LoadBalancers) - 1 {
					log.Fatalln("The http-type load balancer must be the last one")
				}

				for x := range lb.Backends {
					if len(lb.Backends[x].RawURL) == 0 {
						continue
					}

					// log and exit
					if strings.HasPrefix(lb.Backends[x].RawURL, "https://") {
						log.Fatalf("Load Balancer [%s] https-scheme backend is not supported", lb.Name)
					}

					u, err := url.Parse(lb.Backends[x].RawURL)
					// parse error or prefix is missing
					if err != nil || u.Host == "" {
						lb.Backends[x].RawURL = ""
					} else {
						lb.Backends[x].Address = u.Hostname()
						// standard http port
						if len(u.Port()) == 0 {
							lb.Backends[x].Port = 80
						} else {
							// non-standard http(s) port
							port, _ := strconv.Atoi(u.Port())
							lb.Backends[x].Port = port
						}
					}
				}
			}

			// duplicate removal judgment map
			existMap := make(map[string]string, 0)

			// non-repetitive backends slice
			backends := make([]kubevip.BackEnd, 0)

			// set backend.port with backendPort
			for x := range lb.Backends {
				// already contain
				if _, ok := existMap[lb.Backends[x].Address]; ok {
					continue
				}
				existMap[lb.Backends[x].Address] = lb.Backends[x].Address

				// not set alone
				if lb.Backends[x].Port == 0 {
					log.Debugf("Load Balancer [%s] backend [%s] use default backendPort [%d]", lb.Name, lb.Backends[x].Address, backendPort)
					lb.Backends[x].Port = backendPort
				}

				if strings.ToLower(lb.Type) == "http" && len(lb.Backends[x].RawURL) == 0 {
					log.Debugf("Load Balancer [%s] backend [%s] structure rawURL", lb.Name, lb.Backends[x].Address)
					lb.Backends[x].RawURL = fmt.Sprintf("http://%s:%d/", lb.Backends[x].Address, lb.Backends[x].Port)
				}

				backends = append(backends, kubevip.BackEnd{
					Alive:    true,
					Address:  lb.Backends[x].Address,
					Port:     lb.Backends[x].Port,
					RawURL:   lb.Backends[x].RawURL,
				})
			}

			// reassignment of non-repetitive backends
			lb.Backends = backends
		}

		if log.GetLevel() >= log.DebugLevel {
			config, _ := yaml.Marshal(startConfig)
			log.Debugln("Effective [config.yaml]")
			// for log output orderly
			time.Sleep(time.Millisecond * 500)
			fmt.Println(string(config))
			time.Sleep(time.Millisecond * 500)
		}

		var newCluster *cluster.Cluster

		if startConfig.SingleNode {
			// If the Virtual IP isn't disabled then create the netlink configuration
			newCluster, err = cluster.InitCluster(&startConfig, disableVIP)
			if err != nil {
				log.Fatalf("%v", err)
			}
			// Start a single node cluster
			newCluster.StartSingleNode(&startConfig, disableVIP)
		} else {
			if disableVIP {
				log.Fatalln("Cluster mode requires the Virtual IP to be enabled, use single node with no VIP")
			}

			// If the Virtual IP isn't disabled then create the netlink configuration
			newCluster, err = cluster.InitCluster(&startConfig, disableVIP)
			if err != nil {
				log.Fatalf("%v", err)
			}

			if startConfig.EnableLeaderElection {
				cm, err := cluster.NewManager(startKubeConfigPath, inCluster)
				if err != nil {
					log.Fatalf("%v", err)
				}

				// Leader Cluster will block
				err = newCluster.StartLeaderCluster(&startConfig, cm)
				if err != nil {
					log.Fatalf("%v", err)
				}
			} else {

				// // Start a multi-node (raft) cluster, this doesn't block so will wait on signal
				err = newCluster.StartRaftCluster(&startConfig)
				if err != nil {
					log.Fatalf("%v", err)
				}
				signalChan := make(chan os.Signal, 1)
				signal.Notify(signalChan, os.Interrupt)

				<-signalChan

				newCluster.Stop()
			}

		}

	},
}
