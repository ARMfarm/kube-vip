package manager

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	dhclient "github.com/digineo/go-dhclient"
	"github.com/kamhlos/upnp"
	"github.com/plunder-app/kube-vip/pkg/bgp"
	"github.com/plunder-app/kube-vip/pkg/cluster"
	"github.com/plunder-app/kube-vip/pkg/kubevip"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const plunderLock = "plndr-svcs-lock"

var signalChan chan os.Signal

// OutSideCluster allows the controller to be started using a local kubeConfig for testing
var OutSideCluster bool

// Manager degines the manager of the load-balancing services
type Manager struct {
	clientSet *kubernetes.Clientset
	configMap string
	config    *kubevip.Config

	// Manager services
	service bool

	// Keeps track of all running instances
	serviceInstances []Instance

	// Additional functionality
	upnp *upnp.Upnp
	//BGP Manager, this is a singleton that manages all BGP advertisements
	bgpServer *bgp.Server
}

type dhcpService struct {
	// dhcpClient (used DHCP for the vip)
	dhcpClient    *dhclient.Client
	dhcpInterface string
}

// Instance defines an instance of everything needed to manage a vip
type Instance struct {
	// Virtual IP / Load Balancer configuration
	vipConfig kubevip.Config

	// cluster instance
	cluster cluster.Cluster

	// Custom settings
	dhcp *dhcpService

	// Kubernetes service mapping
	Vip  string
	Port int32
	UID  string
	Type string

	ServiceName string
}

// // TODO - call from a package (duplicated struct in the cloud-provider code)
// type service struct {
// 	Vip  string `json:"vip"`
// 	Port int32  `json:"port"`
// 	UID  string `json:"uid"`
// 	Type string `json:"type"`

// 	ServiceName string `json:"serviceName"`
// }

// SetControlPane determines if the control plane should be enabled
// func (sm *Manager) SetControlPane(enable bool) {
// 	sm.controlPane = enable
// }

// New will create a new managing object
func New(configMap string, config *kubevip.Config) (*Manager, error) {
	var clientset *kubernetes.Clientset
	if OutSideCluster == false || !config.EnableControlPane {
		// This will attempt to load the configuration when running within a POD
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("error creating kubernetes client config: %s", err.Error())
		}
		clientset, err = kubernetes.NewForConfig(cfg)

		if err != nil {
			log.Debugln("Using incluster Kubernetes configuration")
			return nil, fmt.Errorf("error creating kubernetes client: %s", err.Error())
		}
		// use the current context in kubeconfig
	} else {
		// Check for file existing
		// First for default path on control plane
		// /etc/kubernetes/admin.conf
		var configPath string
		var cfg *rest.Config
		var err error

		configPath = "/etc/kubernetes/admin.conf"
		if fileExists(configPath) {
			cfg, err = clientcmd.BuildConfigFromFlags("", configPath)
			if err != nil {
				return nil, err
			}
		} else {
			// Second check in home directory for kube config
			configPath = filepath.Join(os.Getenv("HOME"), ".kube", "config")
			cfg, err = clientcmd.BuildConfigFromFlags("", configPath)
			if err != nil {
				return nil, err
			}
		}

		log.Debugf("Using outside Kubernetes configuration from file [%s]", configPath)
		clientset, err = kubernetes.NewForConfig(cfg)

		// If this is a control pane host it will likely have started as a static pod or wont have the
		// VIP up before trying to connect to the API server, we set the API endpoing to this machine to
		// ensure connectivity.
		if config.EnableControlPane {
			// We modify the config so that we can always speak to the correct host
			id, err := os.Hostname()
			if err != nil {
				return nil, err
			}
			cfg.Host = fmt.Sprintf("%s:%v", id, config.Port)
			clientset, err = kubernetes.NewForConfig(cfg)
		}
		if err != nil {
			return nil, fmt.Errorf("error creating kubernetes client: %s", err.Error())
		}
	}

	return &Manager{
		clientSet: clientset,
		configMap: configMap,
		config:    config,
	}, nil
}

// Start will begin the Manager, which will start services and watch the configmap
func (sm *Manager) Start() error {

	// If BGP is enabled then we start a server instance that will broadcast VIPs
	if sm.config.EnableBGP {
		log.Infoln("Starting Kube-vip loadBalancer Service with the BGP engine")
		log.Infof("Namespace [%s], Hybrid mode [%t]", sm.config.Namespace, sm.config.EnableControlPane)
		return sm.startBGP()
	}

	// If ARP is enabled then we start a LeaderElection that will use ARP to advertise VIPs
	if sm.config.EnableARP {
		log.Infoln("Starting loadBalancer Service with the ARP engine")
		log.Infof("Namespace [%s], Hybrid mode [%t]", sm.config.Namespace, sm.config.EnableControlPane)
		return sm.startARP()
	}

	log.Infoln("Prematurely exiting Load-balancer as neither Layer2 or Layer3 is enabled")
	return nil
}

func returnNameSpace() (string, error) {
	if data, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if ns := strings.TrimSpace(string(data)); len(ns) > 0 {
			return ns, nil
		}
		return "", err
	}
	return "", fmt.Errorf("Unable to find Namespace")
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
