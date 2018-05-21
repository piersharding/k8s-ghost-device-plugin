package main

import (
	"flag"
	"os"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/fsnotify/fsnotify"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1beta1"
)

var (
	resourceConfigs string = ""
	resourceName    string = ""
)

func main() {
	// Parse command-line arguments
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	// flagMasterNetDev := flag.String("master", "", "Master ethernet network device for SRIOV, ex: eth1.")
	flagLogLevel := flag.String("log-level", "info", "Define the logging level: error, info, debug.")
	flagResourceName := flag.String("resource-name", defaultResourceName, "Define the default resource name: ska-sdp.org/widget.")
	flagResourceConfigs := flag.String("resource-configfile", defaultResourceConfigFile, "Specify the resource config file location.")
	flag.Parse()

	switch *flagLogLevel {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	}

	if *flagResourceConfigs != "" {
		resourceConfigs = *flagResourceConfigs
	}
	resourceName = *flagResourceName
	log.Debugf("Config file: %s", resourceConfigs)
	log.Debugf("Resource Name: %s", resourceName)

	log.Infof("Fetching devices.")

	devList, err := GetWidgetDevices(resourceConfigs)
	if err != nil {
		log.Errorf("Error to get IB device: %v", err)
		return
	}
	if len(devList) == 0 {
		log.Println("No devices found.")
		return
	}

	log.Debugf("Widget device list: %v", devList)
	log.Println("Starting FS watcher.")
	watcher, err := newFSWatcher(pluginapi.DevicePluginPath)
	if err != nil {
		log.Println("Failed to created FS watcher.")
		os.Exit(1)
	}
	defer watcher.Close()

	log.Println("Starting OS watcher.")
	sigs := newOSWatcher(syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	restart := true
	var devicePlugin *WidgetDevicePlugin

L:
	for {
		if restart {
			if devicePlugin != nil {
				devicePlugin.Stop()
			}

			devicePlugin = NewWidgetDevicePlugin(resourceConfigs, resourceName)
			if err := devicePlugin.Serve(resourceName); err != nil {
				log.Println("Could not contact Kubelet, retrying. Did you enable the device plugin feature gate?")
			} else {
				restart = false
			}
		}

		select {
		case event := <-watcher.Events:
			if event.Name == pluginapi.KubeletSocket && event.Op&fsnotify.Create == fsnotify.Create {
				log.Printf("inotify: %s created, restarting.", pluginapi.KubeletSocket)
				restart = true
			}

		case err := <-watcher.Errors:
			log.Printf("inotify: %s", err)

		case s := <-sigs:
			switch s {
			case syscall.SIGHUP:
				log.Println("Received SIGHUP, restarting.")
				restart = true
			default:
				log.Printf("Received signal \"%v\", shutting down.", s)
				devicePlugin.Stop()
				break L
			}
		}
	}
}
