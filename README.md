# sts-plus-operator
The sts plus operator add features to the standard `StatefulSet` resource.

At the moment the only feature added is the rollout in phases.

## Phased Rollout

The `PhasedRollout` custom resource premits the rolling update of statefulsets in a controlled fashion.
On top of the standard rolling update, at each pod update the procedure is put on hold until several prometheus checks are performed and only of they pass then the rollout will continue to the next pod.
This is a sort of canary release similar to what [flagger](https://flagger.app/) do with deployments (please note that flagger does not support statefulsets).
This is helpful not only to carefully perform deploys but also to stop or slow down deploys when external factors are not met, for example, a rolling update of a postgres cluster should be temporary stopped if a backup is ongoing, or a kafka cluster should temporary stop upgrades if the consumers are delayed to avoud further disruption.

A statefulset can be managed only if the `updateStrategy` is of type `RollingUpdate` (i.e. _not_ `OnDelete`).

The rollout is controlled and temporary put on hold using the `statefulset.spec.updateStrategy.rollingUpdate.partition` field. When there is no need for a rolling update, the partition value is set to the number of replicas of the statefulset, to precent uncontrolled rollout. When the statefulset revision is updated and there is a need to perform a rolling update, for each pod to roll the phased rollout procedure will:
* ensure the pod previous rolled pod is updated;
* ensure all pods in the statefulset are available;
* wait a certain time to allow prometheus to gather fresh data;
* perform prometheus checks, delayed by an interval: after a number of consecutive succesful checks, the rollout is considered safe to continue
* if the rollout is considered safe to continue, do so decreasing `statefulset.spec.updateStrategy.rollingUpdate.partition` to allow the normal rolling update procedure to continue.

Even if `sts.spec.updateStrategy.rollingUpdate.maxUnavailable` is respected, pods will be rolled one at a time. The normal rolling update checks are not superseded by the phased rollout checks (i.e. `sts.spec.minReadySeconds` and readiness probes will be respected).

Prometheus checks semantic is similar to prometheus alerts: if data is returned then the check is considered failed, if no data is returned the check is considered successful.

Example:
```yaml
apiVersion: sts.plus/v1alpha1
kind: PhasedRollout
metadata:
  name: phasedrollout-sample
spec:
  targetRef: web   #the name of the statefulset to manage
  skipCheck: false #if true, the normal rolling update will be resumed
  check:
    initialDelaySeconds: 30 #initial delay to wait after a pod is rolled, to permit prometheus to get fresh data
    periodSeconds: 30       #
    maxTries: 10
    successThreshold: 3
    query:
      expr: "up{}"           # prometheus query 
      url: prom:9000/xx      # url of the prometheus endpoint (i.e. without the `/api/v1/query` path)
      secretRef: prom-secret # optional secret to authenticate to prometheus
--
apiVersion: v1
stringData:
  token: token #optional token for bearer authentication
  password: password #optional username to use for basic authentication
  username: username #optional password to use for basic authentication
kind: Secret
metadata:
  name: prom-secret
type: Opaque

```

## Getting Started
You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### Running on the cluster
1. Install Instances of Custom Resources:

```sh
kubectl apply -f config/samples/
```

2. Build and push your image to the location specified by `IMG`:
	
```sh
make docker-build docker-push IMG=<some-registry>/sts-plus-operator:tag
```
	
3. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/sts-plus-operator:tag
```

### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller to the cluster:

```sh
make undeploy
```

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/) 
which provides a reconcile function responsible for synchronizing resources untile the desired state is reached on the cluster 

### Test It Out
1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.


the operator will take ownership of statefulset.spec.updateStrategy.rollingUpdate.partition
