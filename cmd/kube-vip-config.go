package cmd

import (
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/ghodss/yaml"
	"github.com/plunder-app/kube-vip/pkg/kubevip"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	appv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// [sample configuration] - flags
var cliConfig kubevip.Config
var cliConfigLB kubevip.LoadBalancer
var cliLocalPeer string
var cliRemotePeers, cliBackends []string
var priority int32 = 2000001000
var hostPathFile = appv1.HostPathFile

func init() {
	kubeVipSampleConfig.Flags().StringVar(&cliConfig.Interface, "interface", "eth0", "Name of the interface to bind to")
	kubeVipSampleConfig.Flags().StringVar(&cliConfig.VIP, "vip", "192.168.0.1", "The Virtual IP address")
	kubeVipSampleConfig.Flags().BoolVar(&cliConfig.SingleNode, "singleNode", false, "Start this instance as a single node")
	kubeVipSampleConfig.Flags().BoolVar(&cliConfig.StartAsLeader, "startAsLeader", false, "Start this instance as the cluster leader")
	kubeVipSampleConfig.Flags().BoolVar(&cliConfig.GratuitousARP, "arp", true, "Use ARP broadcasts to improve VIP re-allocations")
	kubeVipSampleConfig.Flags().StringVar(&cliLocalPeer, "localPeer", "server1:192.168.0.1:10000", "Settings for this peer, format: id:address:port")
	kubeVipSampleConfig.Flags().StringSliceVar(&cliRemotePeers, "remotePeers", []string{"server2:192.168.0.2:10000", "server3:192.168.0.3:10000"}, "Comma seperated remotePeers, format: id:address:port")
	// Load Balancer flags
	kubeVipSampleConfig.Flags().BoolVar(&cliConfigLB.BindToVip, "lbBindToVip", false, "Bind example load balancer to VIP")
	kubeVipSampleConfig.Flags().StringVar(&cliConfigLB.Type, "lbType", "tcp", "Type of load balancer instance (TCP/HTTP)")
	kubeVipSampleConfig.Flags().StringVar(&cliConfigLB.Name, "lbName", "Example Load Balancer", "The name of a load balancer instance")
	kubeVipSampleConfig.Flags().IntVar(&cliConfigLB.Port, "lbPort", 6444, "Port that load balancer will expose on")
	kubeVipSampleConfig.Flags().IntVar(&cliConfigLB.BackendPort, "lbBackEndPort", 6443, "A port that all backends may be using (optional)")
	kubeVipSampleConfig.Flags().StringSliceVar(&cliBackends, "lbBackends", []string{"192.168.0.1:6443", "192.168.0.2:6443"}, "Comma seperated backends, format: address:port")
}

var kubeVipSampleConfig = &cobra.Command{
	Use:   "config",
	Short: "Generate a Sample configuration",
	Run: func(cmd *cobra.Command, args []string) {

		// // Parse localPeer
		// p, err := kubevip.ParsePeerConfig(cliLocalPeer)
		// if err != nil {
		// 	cmd.Help()
		// 	log.Fatalln(err)
		// }
		// cliConfig.LocalPeer = *p

		// // Parse remotePeers
		// //Iterate backends
		// for i := range cliRemotePeers {
		// 	p, err := kubevip.ParsePeerConfig(cliRemotePeers[i])
		// 	if err != nil {
		// 		cmd.Help()
		// 		log.Fatalln(err)
		// 	}
		// 	cliConfig.RemotePeers = append(cliConfig.RemotePeers, *p)
		// }

		// //Iterate backends
		// for i := range cliBackends {
		// 	b, err := kubevip.ParseBackendConfig(cliBackends[i])
		// 	if err != nil {
		// 		cmd.Help()
		// 		log.Fatalln(err)
		// 	}
		// 	cliConfigLB.Backends = append(cliConfigLB.Backends, *b)
		// }

		// Add the basic Load-Balancer to the configuration
		cliConfig.LoadBalancers = append(cliConfig.LoadBalancers, cliConfigLB)

		err := cliConfig.ParseFlags(cliLocalPeer, cliRemotePeers, cliBackends)
		if err != nil {
			cmd.Help()
			log.Fatalln(err)
		}

		err = kubevip.ParseEnvironment(&cliConfig)
		if err != nil {
			cmd.Help()
			log.Fatalln(err)
		}

		cliConfig.PrintConfig()
	},
}

var kubeVipSampleManifest = &cobra.Command{
	Use:   "manifest",
	Short: "Generate a Sample kubernetes manifest",
	Run: func(cmd *cobra.Command, args []string) {
		// Generate the sample manifest specification
		p := &appv1.Pod{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Pod",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kube-vip",
				Namespace: "kube-system",
				Labels:    map[string]string{
					"component": "kube-vip",
					"tier": "control-plane",
				},
			},
			Spec: appv1.PodSpec{
				Containers: []appv1.Container{
					{
						Name:  "kube-vip",
						Image: fmt.Sprintf("docker.io/shilazi/kube-vip:%s", Release.Version),
						SecurityContext: &appv1.SecurityContext{
							Capabilities: &appv1.Capabilities{
								Add: []appv1.Capability{
									"NET_ADMIN",
									"SYS_TIME",
								},
							},
						},
						Args: []string{
							"--log=4",
							"start",
							"-c",
							"/etc/kube-vip/config.yaml",
						},
						Resources: appv1.ResourceRequirements{
							Requests: appv1.ResourceList{
								appv1.ResourceCPU: resource.MustParse("250m"),
							},
						},
						VolumeMounts: []appv1.VolumeMount{
							{
								Name:      "config",
								MountPath: "/etc/kube-vip/config.yaml",
							},
						},
					},
				},
				Volumes: []appv1.Volume{
					{
						Name: "config",
						VolumeSource: appv1.VolumeSource{
							HostPath: &appv1.HostPathVolumeSource{
								Path: "/etc/kubernetes/vip.conf",
								Type: &hostPathFile,
							},
						},
					},
				},
				HostNetwork: true,
				Priority: &priority,
				PriorityClassName: "system-node-critical",
				SecurityContext: &appv1.PodSecurityContext {
					SeccompProfile: & appv1.SeccompProfile{
						Type: appv1.SeccompProfileTypeRuntimeDefault,
					},
				},
			},
		}

		b, _ := yaml.Marshal(p)
		fmt.Printf(string(b))
	},
}
