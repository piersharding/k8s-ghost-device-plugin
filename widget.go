package main

import (
	"encoding/json"
	"flag"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/piersharding/k8s-ghost-device-plugin/file"
	"golang.org/x/net/context"
	yaml "gopkg.in/yaml.v2"
	"io/ioutil"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1beta1"
	"os"
	"path/filepath"
	"runtime"
)

const RdmaDeviceRource = "/sys/class/infiniband/%s/device/resource"
const NetDeviceRource = "/sys/class/net/%s/device/resource"

var flagStrictPerms = flag.Bool("strict.perms", true, "Strict permission checking on config files")

func IsStrictPerms() bool {
	if !*flagStrictPerms || os.Getenv("STRICT_PERMS") == "false" {
		return false
	}
	return true
}

// ownerHasExclusiveWritePerms asserts that the current user or root is the
// owner of the config file and that the config file is (at most) writable by
// the owner or root (e.g. group and other cannot have write access).
func ownerHasExclusiveWritePerms(name string) error {
	if runtime.GOOS == "windows" {
		return nil
	}

	info, err := file.Stat(name)
	if err != nil {
		return err
	}

	euid := os.Geteuid()
	fileUID, _ := info.UID()
	perm := info.Mode().Perm()

	if fileUID != 0 && euid != fileUID {
		return fmt.Errorf(`config file ("%v") must be owned by the beat user `+
			`(uid=%v) or root`, name, euid)
	}

	// Test if group or other have write permissions.
	if perm&0022 > 0 {
		nameAbs, err := filepath.Abs(name)
		if err != nil {
			nameAbs = name
		}
		return fmt.Errorf(`config file ("%v") can only be writable by the `+
			`owner but the permissions are "%v" (to fix the permissions use: `+
			`'chmod go-w %v')`,
			name, perm, nameAbs)
	}

	return nil
}

// widget.devices:
// - type: snaffler
//   model: v1
//   device: /dev/wibble1

type WidgetConfig struct {
	Devices []struct {
		Type   string
		Model  string
		Device string
	}
}

func LoadFile(path string) (*WidgetConfig, error) {
	if IsStrictPerms() {
		if err := ownerHasExclusiveWritePerms(path); err != nil {
			return nil, err
		}
	}
	var cfg WidgetConfig
	reader, _ := os.Open(path)
	buf, _ := ioutil.ReadAll(reader)
	err := yaml.Unmarshal(buf, &cfg)
	if err != nil {
		log.Errorf("Error parsing YAML config: %v", err)
		return nil, err
	}
	cfgOut, _ := json.Marshal(cfg)
	log.Debugf("loading config from file '%v' => %v", path, string(cfgOut))
	return &cfg, err
}

func GetWidgetDevices(resourceConfigs string) ([]Device, error) {

	log.Debugf("Going to read config")

	cfg, err := LoadFile(resourceConfigs)
	if err != nil {
		log.Errorf("Error reading config: %v", err)
		return nil, err
	}

	return generateWidgetDevices(*cfg)
}

func generateWidgetDevices(widgetConfigs WidgetConfig) ([]Device, error) {
	var devs []Device
	// Get all RDMA device list

	for i, w := range widgetConfigs.Devices {
		devs = append(devs, Device{
			// RdmaDevice: d,
			// NetDevice:   n,
			Id:           i, // starts at 0
			Name:         fmt.Sprintf("%s_%s_%d", w.Type, w.Model, i),
			DeviceType:   w.Type,
			DeviceModel:  w.Model,
			WidgetDevice: w.Device,
		})
	}

	// ibvDevList, err := ibverbs.IbvGetDeviceList()
	// if err != nil {
	// 	return nil, err
	// }

	// netDevList, err := GetVfNetDevice(resourceConfigs)
	// if err != nil {
	// 	return nil, err
	// }

	// for _, d := range ibvDevList {
	// 	for _, n := range netDevList {
	// 		dResource, err := getRdmaDeviceResoure(d.Name)
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		nResource, err := getNetDeviceResoure(n)
	// 		if err != nil {
	// 			return nil, err
	// 		}

	// 		// the same device
	// 		if bytes.Compare(dResource, nResource) == 0 {
	// 			devs = append(devs, Device{
	// 				RdmaDevice: d,
	// 				NetDevice:  n,
	// 			})
	// 		}
	// 	}
	// }
	return devs, nil
}

// func getRdmaDeviceResoure(name string) ([]byte, error) {
// 	resourceFile := fmt.Sprintf(RdmaDeviceRource, name)
// 	data, err := ioutil.ReadFile(resourceFile)
// 	return data, err
// }

// func getNetDeviceResoure(name string) ([]byte, error) {
// 	resourceFile := fmt.Sprintf(NetDeviceRource, name)
// 	data, err := ioutil.ReadFile(resourceFile)
// 	return data, err
// }

func deviceExists(devs []*pluginapi.Device, id string) bool {
	for _, d := range devs {
		if d.ID == id {
			return true
		}
	}
	return false
}

func watchXIDs(ctx context.Context, devs []*pluginapi.Device, xids chan<- *pluginapi.Device) {
	for {
		select {
		case <-ctx.Done():
			return
		}

		// TODO: check Widget device healthy status
	}
}
