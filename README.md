# Kubernetes Event Collector

The Kubernetes Event Collector (KEL) watches for Kubernetes events within a namespace
and stores them to a buffer which can be dumped. Kubernetes events are useful
for monitoring the state of a cluster and applications within a cluster however
they are only stored by Kubernetes for a limited period of time and they can also
be noisy if multiple applications are running within a namespace. KEL watches 
events and stores them in a buffer which can be dumped when something goes wrong.

## Installation

KEL can be installed into a kubernetes namespace using the provided helm charts 
in the  `event-collector/` directory

```
helm install event-collector charts/event-collector
```

## API

A REST API can be used to communicate with a KEL instance to view dumps taken
as well as triggger new dumps. By default it is served on port 8080

* List all taken dumps

    `GET /dumps`

* Trigger a dump

    `POST /dumps`

* Get a dump

    `GET /dumps/<dump_name`

* Get the current buffer

    `GET /buffer`

## Configuration
The Event Collector is configured using a /etc/eventcollector/config.yaml file. 

### Dump completion plugins
Dump completion plugins trigger an action when a dump is taken. Currently supported plugins
* KubernetesEvents

In the future we plan to support Slack as-well

### Event Filters
Simple filters can be set using the config file to filter events by:
* Involved Object API Version
* Involved Object Kind
* Involved Object Labels


### Example 
Example Config file
```
bufferSize: 1000                # The buffer size determines how many events are stored
dumpCompletionPlugins:          # Plugins to trigger actions when a dump is taken 
  kubernetesEvent:
    enabled: true               # Enable creating a K8s event when a dump is taken
dumpOnWarningEvents: true       # Will trigger dumps when the collector sees an event type warning
eventFilters:                   # Filters for events to be collected, if an event matches any of the filters it will be collected
- apiVersion: couchbase.com/v2  # This filter will collect any events with involvedObjects of "couchbase.com/v2" API version
- labels:                       # This filter will collect any event where the involvedObjects labels have app=couchbase AND couchbase_server=true*
  - key: app
    value: couchbase
  - key: couchbase_server
    value: true
- labels                        # This filter will collect any event where the involvedObjects labels have app=couchbase-operator*
  - key: app
    value: couchbase-operator    
```

*: Label matching is currently limited only to Pods, Deployments and
