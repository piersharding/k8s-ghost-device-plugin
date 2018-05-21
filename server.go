package main

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1beta1"
)

const (
	defaultResourceName       = "ska-sdp.org/widget"
	defaultResourceConfigFile = "/etc/kubernetes/widget.yml"
	serverSock                = pluginapi.DevicePluginPath + "%s_widget.sock"
)

// WidgetDevicePlugin implements the Kubernetes device plugin API
type WidgetDevicePlugin struct {
	devs   []*pluginapi.Device
	socket string
	// masterNetDevice string
	// ID => Device
	devices map[string]Device
	stop    chan interface{}
	health  chan *pluginapi.Device
	server  *grpc.Server
}

// NewWidgetDevicePlugin returns an initialized WidgetDevicePlugin
func NewWidgetDevicePlugin(resourceConfigs string, resourceName string) *WidgetDevicePlugin {
	log.Debugf("other instance of GetWidgetDevices")
	devices, err := GetWidgetDevices(resourceConfigs)
	if err != nil {
		log.Errorf("Error detecting widget devices: %v", err)
		return nil
	}

	// because we can run multiple instances of this plugin
	// we need to uniquely name the socket for each
	encodedResourceName := base64.StdEncoding.EncodeToString([]byte(resourceName))
	log.Debugf("Base64 encoded Resource Name: %s", encodedResourceName)

	var devs []*pluginapi.Device
	devMap := make(map[string]Device)
	for _, device := range devices {
		// id := device.RdmaDevice.Name
		id := device.Name
		devs = append(devs, &pluginapi.Device{
			ID:     id,
			Health: pluginapi.Healthy,
		})
		devMap[id] = device
	}

	return &WidgetDevicePlugin{
		// masterNetDevice: resourceConfigs,
		socket:  fmt.Sprintf(serverSock, encodedResourceName),
		devs:    devs,
		devices: devMap,
		stop:    make(chan interface{}),
		health:  make(chan *pluginapi.Device),
	}
}

func (m *WidgetDevicePlugin) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

// dial establishes the gRPC communication with the registered device plugin.
func dial(unixSocketPath string, timeout time.Duration) (*grpc.ClientConn, error) {
	c, err := grpc.Dial(unixSocketPath, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithTimeout(timeout),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)

	if err != nil {
		return nil, err
	}

	return c, nil
}

// Start starts the gRPC server of the device plugin
func (m *WidgetDevicePlugin) Start() error {
	err := m.cleanup()
	if err != nil {
		return err
	}

	sock, err := net.Listen("unix", m.socket)
	if err != nil {
		return err
	}

	m.server = grpc.NewServer([]grpc.ServerOption{}...)
	pluginapi.RegisterDevicePluginServer(m.server, m)

	go m.server.Serve(sock)

	// Wait for server to start by launching a blocking connection
	conn, err := dial(m.socket, 5*time.Second)
	if err != nil {
		return err
	}
	conn.Close()

	go m.healthcheck()

	return nil
}

// Stop stops the gRPC server
func (m *WidgetDevicePlugin) Stop() error {
	if m.server == nil {
		return nil
	}

	m.server.Stop()
	m.server = nil
	close(m.stop)

	return m.cleanup()
}

// Register registers the device plugin for the given resourceName with Kubelet.
func (m *WidgetDevicePlugin) Register(kubeletEndpoint, resourceName string) error {
	conn, err := dial(kubeletEndpoint, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pluginapi.NewRegistrationClient(conn)
	reqt := &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     path.Base(m.socket),
		ResourceName: resourceName,
	}

	_, err = client.Register(context.Background(), reqt)
	if err != nil {
		return err
	}
	return nil
}

// ListAndWatch lists devices and update that list according to the health status
func (m *WidgetDevicePlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devs})

	for {
		select {
		case <-m.stop:
			return nil
		case d := <-m.health:
			// FIXME: there is no way to recover from the Unhealthy state.
			d.Health = pluginapi.Unhealthy
			s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devs})
		}
	}
}

func (m *WidgetDevicePlugin) unhealthy(dev *pluginapi.Device) {
	m.health <- dev
}

// Allocate which return list of devices.
func (m *WidgetDevicePlugin) Allocate(ctx context.Context, reqs *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	devs := m.devs
	responses := pluginapi.AllocateResponse{}

	for _, req := range reqs.ContainerRequests {
		log.Debugf("Request IDs: %v", req.DevicesIDs)
		dev_names := []string{}
		for _, id := range req.DevicesIDs {
			if !deviceExists(devs, id) {
				return nil, fmt.Errorf("invalid allocation request: unknown device: %s", id)
			}
			dev_names = append(dev_names, m.devices[id].WidgetDevice)
		}
		log.Debugf("Devices to be offered: %v/%v", req.DevicesIDs, dev_names)
		response := pluginapi.ContainerAllocateResponse{
			Envs: map[string]string{
				"WIDGET_VISIBLE_DEVICE_IDS": strings.Join(req.DevicesIDs, ","),
				"WIDGET_VISIBLE_DEVICES":    strings.Join(dev_names, ","),
			},
		}

		// for _, id := range req.DevicesIDs {
		// 	if !deviceExists(devs, id) {
		// 		return nil, fmt.Errorf("invalid allocation request: unknown device: %s", id)
		// 	}
		// }

		responses.ContainerResponses = append(responses.ContainerResponses, &response)
	}

	// var devicesList []*pluginapi.DeviceSpec
	// for _, id := range r.DevicesIDs {
	// 	if !deviceExists(devs, id) {
	// 		return nil, fmt.Errorf("invalid allocation request: unknown device: %s", id)
	// 	}

	// 	var devPath string
	// 	if dev, ok := m.devices[id]; ok {
	// 		// TODO: to function
	// 		// devPath = fmt.Sprintf("/dev/infiniband/%s", dev.RdmaDevice.DevName)
	// 		devPath = dev.WidgetDevice
	// 	} else {
	// 		continue
	// 	}

	// 	ds := &pluginapi.DeviceSpec{
	// 		ContainerPath: devPath,
	// 		HostPath:      devPath,
	// 		Permissions:   "rw",
	// 	}
	// 	devicesList = append(devicesList, ds)
	// }

	// response.Devices = devicesList

	return &responses, nil
}

func (m *WidgetDevicePlugin) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (m *WidgetDevicePlugin) cleanup() error {
	if err := os.Remove(m.socket); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (m *WidgetDevicePlugin) healthcheck() {
	ctx, cancel := context.WithCancel(context.Background())

	xids := make(chan *pluginapi.Device)
	go watchXIDs(ctx, m.devs, xids)

	for {
		select {
		case <-m.stop:
			cancel()
			return
		case dev := <-xids:
			m.unhealthy(dev)
		}
	}
}

// Serve starts the gRPC server and register the device plugin to Kubelet
func (m *WidgetDevicePlugin) Serve(resourceName string) error {
	err := m.Start()
	if err != nil {
		log.Errorf("Could not start device plugin: %v", err)
		return err
	}
	log.Infof("Starting to serve on %s", m.socket)

	err = m.Register(pluginapi.KubeletSocket, resourceName)
	if err != nil {
		log.Errorf("Could not register device plugin: %v", err)
		m.Stop()
		return err
	}
	log.Infof("Registered device plugin with Kubelet")

	return nil
}
