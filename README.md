# Widget device plugin for Kubernetes ![](https://github.com/piersharding/k8s-ghost-device-plugin)

Based on the original work of https://github.com/hustcat/k8s-rdma-device-plugin

## Introduction

`k8s-widget-device-plugin` is a [device plugin](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/resource-management/device-plugin.md) for Kubernetes to create mock devices for scheduling testing purposes.


## Quick Start

### Build

Relies on vndr (https://github.com/LK4D4/vndr), which can be installed with `go get github.com/LK4D4/vndr`, and must be found in $PATH.

Run `build`:

```
# make all 
# ls bin
k8s-ghost-device-plugin
```

### Work with Kubernetes

* Preparing Ghost node

Start `kubelet` with `--feature-gates=DevicePlugins=true`.

* Create a resource config file:
```
devices:
- type: snaffler
  model: v1
  device: /dev/wibble1  
- type: snaffler
  model: v1
  device: /dev/wibble2  
- type: snaffler
  model: v2
  device: /dev/wibble3  
```

* Run device plugin daemon process

```
# bin/k8s-rdma-device-plugin  -resource-configfile ./widget.yml -log-level debug
DEBU[0000] Config file: /etc/kubernetes/widget.yml      
DEBU[0000] Resource Name: ska-sdp.org/widget            
INFO[0000] Fetching devices.                            
DEBU[0000] Going to read config                         
DEBU[0000] loading config from file '/etc/kubernetes/widget.yml' => {"Devices":[{"Type":"snaffler","Model":"v1","Device":"/dev/wibble1"},{"Type":"snaffler","Model":"v1","Device":"/dev/wibble2"},{"Type":"snaffler","Model":"v2","Device":"/dev/wibble3"}]} 
DEBU[0000] Widget device list: [{v1 snaffler 0 snaffler_v1_0 /dev/wibble1} {v1 snaffler 1 snaffler_v1_1 /dev/wibble2} {v2 snaffler 2 snaffler_v2_2 /dev/wibble3}] 
INFO[0000] Starting FS watcher.                         
INFO[0000] Starting OS watcher.                         
DEBU[0000] other instance of GetWidgetDevices           
DEBU[0000] Going to read config                         
DEBU[0000] loading config from file '/etc/kubernetes/widget.yml' => {"Devices":[{"Type":"snaffler","Model":"v1","Device":"/dev/wibble1"},{"Type":"snaffler","Model":"v1","Device":"/dev/wibble2"},{"Type":"snaffler","Model":"v2","Device":"/dev/wibble3"}]} 
DEBU[0000] Base64 encoded Resource Name: c2thLXNkcC5vcmcvd2lkZ2V0 
INFO[0000] Starting to serve on /var/lib/kubelet/device-plugins/c2thLXNkcC5vcmcvd2lkZ2V0_widget.sock 
INFO[0000] Registered device plugin with Kubelet   
...
```

or deploy it as a daemonset:

```
# kubectl -n kube-system apply -f ghost-device-plugin.yml
# kubectl -n kube-system get pods
ghost-device-plugin-daemonset-2wbdv         1/1       Running   0          14m
ghost-device-plugin-daemonset-7pwf7         1/1       Running   0          14m
```
* List out the allocatable resource, and capacity per node
```
kubectl get nodes -o json | jq     '.items[] | .metadata.name, .status.allocatable, .status.capacity'
```
Which gives:
```
"k8s-master-0"
{
  "cpu": "64",
  "ephemeral-storage": "337894066846",
  "hugepages-1Gi": "0",
  "hugepages-2Mi": "0",
  "memory": "131813436Ki",
  "pods": "110",
  "ska-sdp.org/widget": "0"  # not scheduled on master
}
"k8s-minion-0"
{
  "cpu": "64",
  "ephemeral-storage": "337894066846",
  "hugepages-1Gi": "0",
  "hugepages-2Mi": "0",
  "memory": "131813436Ki",
  "pods": "110",
  "ska-sdp.org/widget": "3"
}
...
```

* Run Test container

```
apiVersion: apps/v1beta2
kind: Deployment
metadata:
  name: ghost-container
  namespace: default
spec:
  selector:
    matchLabels:
      k8s-app: ghost-container
  replicas: 1
  template:
    metadata:
      labels:
        k8s-app: ghost-container
    spec:
      containers:
      - name: ghost-container
        image: busybox:latest
        resources:
          limits:
            ska-sdp.org/widget: 1 # requesting 1 ghost device
        securityContext:
          capabilities:
            add: ["ALL"]
        command:
          - env;
          - /bin/sleep
          - "360000"
```

## TODO

### Run multiple types of mock devices 

