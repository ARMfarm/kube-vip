package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/plunder-app/kube-vip/pkg/kubevip"
	"github.com/plunder-app/kube-vip/pkg/service"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// Path to the configuration file
var configPath string

// Disable the Virtual IP (bind to the existing network stack)
var disableVIP bool

// Run as a load balancer service (within a pod / kubernetes)
var serviceArp bool

// ConfigMap name within a Kubernetes cluster
var configMap string

// Configure the level of loggin
var logLevel uint32

// Release - this struct contains the release information populated when building kube-vip
var Release struct {
	Version string
	Build   string
}

// Structs used via the various subcommands
var initConfig kubevip.Config
var initLoadBalancer kubevip.LoadBalancer

// Points to a kubernetes configuration file
var kubeConfigPath string

var kubeVipCmd = &cobra.Command{
	Use:   "kube-vip",
	Short: "This is a server for providing a Virtual IP and load-balancer for the Kubernetes control-plane",
}

func init() {

	localpeer, err := autoGenLocalPeer()
	if err != nil {
		log.Fatalln(err)
	}
	initConfig.LocalPeer = *localpeer
	//initConfig.Peers = append(initConfig.Peers, *localpeer)
	kubeVipCmd.PersistentFlags().StringVar(&initConfig.Interface, "interface", "", "Name of the interface to bind to")
	kubeVipCmd.PersistentFlags().StringVar(&initConfig.VIP, "vip", "", "The Virtual IP address")
	kubeVipCmd.PersistentFlags().StringVar(&initConfig.VIPCIDR, "cidr", "", "The CIDR range for the virtual IP address")
	kubeVipCmd.PersistentFlags().BoolVar(&initConfig.GratuitousARP, "arp", true, "Enable Arp for Vip changes")

	// Clustering type (leaderElection)
	kubeVipCmd.PersistentFlags().BoolVar(&initConfig.EnableLeaderElection, "leaderElection", false, "Use the Kubernetes leader election mechanism for clustering")
	kubeVipCmd.PersistentFlags().IntVar(&initConfig.LeaseDuration, "leaseDuration", 5, "Length of time a Kubernetes leader lease can be held for")
	kubeVipCmd.PersistentFlags().IntVar(&initConfig.RenewDeadline, "leaseRenewDuration", 3, "Length of time a Kubernetes leader can attempt to renew its lease")
	kubeVipCmd.PersistentFlags().IntVar(&initConfig.RetryPeriod, "leaseRetry", 1, "Number of times the host will retry to hold a lease")

	// Clustering type (raft)
	kubeVipCmd.PersistentFlags().BoolVar(&initConfig.StartAsLeader, "startAsLeader", false, "Start this instance as the cluster leader")
	kubeVipCmd.PersistentFlags().BoolVar(&initConfig.AddPeersAsBackends, "addPeersToLB", true, "Add raft peers to the load-balancer")

	// Packet flags
	kubeVipCmd.PersistentFlags().BoolVar(&initConfig.EnablePacket, "packet", false, "This will use the Packet API (requires the token ENV) to update the EIP <-> VIP")
	kubeVipCmd.PersistentFlags().StringVar(&initConfig.PacketAPIKey, "packetKey", "", "The API token for authenticating with the Packet API")
	kubeVipCmd.PersistentFlags().StringVar(&initConfig.PacketProject, "packetProject", "", "The name of project already created within Packet")

	// Load Balancer flags
	kubeVipCmd.PersistentFlags().BoolVar(&initConfig.EnableLoadBalancer, "lbEnable", false, "Enable a load-balancer on the VIP")
	kubeVipCmd.PersistentFlags().BoolVar(&initLoadBalancer.BindToVip, "lbBindToVip", true, "Bind example load balancer to VIP")
	kubeVipCmd.PersistentFlags().StringVar(&initLoadBalancer.Type, "lbType", "tcp", "Type of load balancer instance (TCP/HTTP)")
	kubeVipCmd.PersistentFlags().StringVar(&initLoadBalancer.Name, "lbName", "Kubeadm Load Balancer", "The name of a load balancer instance")
	kubeVipCmd.PersistentFlags().IntVar(&initLoadBalancer.Port, "lbPort", 6443, "Port that load balancer will expose on")
	kubeVipCmd.PersistentFlags().IntVar(&initLoadBalancer.BackendPort, "lbBackEndPort", 6444, "A port that all backends may be using (optional)")

	// BGP flags
	kubeVipCmd.PersistentFlags().BoolVar(&initConfig.EnableBGP, "bgp", false, "This will enable BGP support within kube-vip")
	kubeVipCmd.PersistentFlags().StringVar(&initConfig.BGPConfig.RouterID, "bgpRouterID", "", "The routerID for the bgp server")
	kubeVipCmd.PersistentFlags().Uint32Var(&initConfig.BGPConfig.AS, "localAS", 65000, "The local AS number for the bgp server")
	kubeVipCmd.PersistentFlags().StringVar(&initConfig.BGPPeerConfig.Address, "peerAddress", "", "The address of a BGP peer")
	kubeVipCmd.PersistentFlags().Uint32Var(&initConfig.BGPPeerConfig.AS, "peerAS", 65000, "The AS number for a BGP peer")

	// Manage logging
	kubeVipCmd.PersistentFlags().Uint32Var(&logLevel, "log", 4, "Set the level of logging")

	// Service flags
	kubeVipService.Flags().StringVarP(&configMap, "configMap", "c", "kube-vip", "The configuration map defined within the cluster")
	kubeVipService.Flags().StringVarP(&service.Interface, "interface", "i", "eth0", "Name of the interface to bind to")
	kubeVipService.Flags().BoolVar(&service.OutSideCluster, "OutSideCluster", false, "Start Controller outside of cluster")
	kubeVipService.Flags().BoolVar(&service.EnableArp, "arp", false, "Use ARP broadcasts to improve VIP re-allocations")

	kubeVipCmd.AddCommand(kubeKubeadm)
	kubeVipCmd.AddCommand(kubeManifest)
	kubeVipCmd.AddCommand(kubeVipSample)
	kubeVipCmd.AddCommand(kubeVipService)
	kubeVipCmd.AddCommand(kubeVipStart)
	kubeVipCmd.AddCommand(kubeVipVersion)

	// Sample commands
	kubeVipSample.AddCommand(kubeVipSampleConfig)
	kubeVipSample.AddCommand(kubeVipSampleManifest)

}

// Execute - starts the command parsing process
func Execute() {
	if err := kubeVipCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var kubeVipVersion = &cobra.Command{
	Use:   "version",
	Short: "Version and Release information about the Kubernetes Virtual IP Server",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Kube-VIP Release Information\n")
		fmt.Printf("Version:  %s\n", Release.Version)
		fmt.Printf("Build:    %s\n", Release.Build)
	},
}

var kubeVipSample = &cobra.Command{
	Use:   "sample",
	Short: "Generate a Sample configuration",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var kubeVipService = &cobra.Command{
	Use:   "service",
	Short: "Start the Virtual IP / Load balancer as a service within a Kubernetes cluster",
	Run: func(cmd *cobra.Command, args []string) {
		// Set the logging level for all subsequent functions
		log.SetLevel(log.Level(logLevel))

		// User Environment variables as an option to make manifest clearer
		envInterface := os.Getenv("vip_interface")
		if envInterface != "" {
			service.Interface = envInterface
		}

		envConfigMap := os.Getenv("vip_configmap")
		if envInterface != "" {
			configMap = envConfigMap
		}

		envLog := os.Getenv("vip_loglevel")
		if envLog != "" {
			logLevel, err := strconv.Atoi(envLog)
			if err != nil {
				panic(fmt.Sprintf("Unable to parse environment variable [vip_loglevel], should be int"))
			}
			log.SetLevel(log.Level(logLevel))
		}

		envArp := os.Getenv("vip_arp")
		if envArp != "" {
			arpBool, err := strconv.ParseBool(envArp)
			if err != nil {
				panic(fmt.Sprintf("Unable to parse environment variable [arp], should be bool (true/false)"))
			}
			service.EnableArp = arpBool
		}

		// Define the new service manager
		mgr, err := service.NewManager(configMap)
		if err != nil {
			log.Fatalf("%v", err)
		}
		// Start the service manager, this will watch the config Map and construct kube-vip services for it
		err = mgr.Start()
		if err != nil {
			log.Fatalf("%v", err)
		}
	},
}
