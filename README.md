# Kubernetes Event Collector

The Kubernetes Event Collector (KEL) watches for Kubernetes events within a namespace
and stores them to a buffer which can be stashed. Kubernetes events are useful
for monitoring the state of a cluster and applications within a cluster however
they are only stored by Kubernetes for a limited period of time and they can also
be noisy if multiple applications are running within a namespace. KEL watches 
events and stores them in a buffer which can be stashed when something goes wrong.

## Support
Please note that support for this repository is provided on a best-effort basis. We are committed to assisting you, but responses may vary in timeliness depending on current availability and the complexity of the issue.

Before you proceed:
* Ensure you have reviewed this README for initial troubleshooting steps.
* Search through existing issues to see if your question has been addressed.

How to get help:
* If you have found a bug or need assistance, please open an issue with detailed information.
* For general inquiries or discussions, use the Discussions section.

We appreciate your understanding and patience as we work to resolve your concerns. Your contributions to improving this project are valued!
Happy coding! :rocket:

## Installation

KEL can be installed into a kubernetes namespace using the provided helm charts 
in the  `event-collector/` directory

```
helm install event-collector charts/event-collector
```

## Using with Couchbase Operator
### Configuring
When installed with the default helm configuration KEL will watch all events in the deployed namespace.
If you want to only watch Couchbase specific events that you can install it with the provided Couchbase values file.

```
helm install event-collector  charts/event-collector  --values charts/event-collector/values-couchbase.yaml
```

### Collecting Logs
When you run `cao collect-logs` in a namespace that is running the event-collecor then the `cao` tool will detect it and fetch the logs in the KEL buffer and store it in the collected archive at namespace/<namespace-name>/deployment/event-collector/buffer.json

## API

A REST API can be used to communicate with a KEL instance to view event stashes
taken as well as trigger new stashes. By default it is served on port 8080

* List all taken stash

    `GET /stashes`

* Trigger a stash

    `POST /stashes`

* Get a stash

    `GET /stashes/<stash_name>`

* Get the current buffer

    `GET /buffer`

## Configuration
The Event Collector is configured using a /etc/eventcollector/config.yaml file. 

### Stash completion plugins
Stash completion plugins trigger an action when a stash is taken. Currently supported plugins
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
stashCompletionPlugins:          # Plugins to trigger actions when a stash is taken 
  kubernetesEvent:
    enabled: true               # Enable creating a K8s event when a stash is taken
stashOnWarningEvents: true       # Will trigger a stash when the collector sees an event type warning
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

*: Label matching is currently limited only to Pods, Deployments and PersistentVolumeClaims
