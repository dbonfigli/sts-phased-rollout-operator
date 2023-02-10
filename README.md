# sts-phased-rollout-operator
The sts phased rollout operator manages the update of statefulsets pods in a controlled fashion, on top of the standard rolling update.

During the standard rolling update process, at each pod update, the update process is put on hold until several prometheus checks are performed and only if they are succesful then the rollout  is permitted to continue to the next pod.

This is helpful not only to carefully perform deploys but also to stop or slow down deploys when external factors are not met, for example, a rolling update of an elasticsearch cluster can be temporary stopped if a backup is ongoing, or a kafka cluster should temporary stop upgrades if the consumers are delayed to avoid further disruption.

This is a sort of canary release similar to what [flagger](https://flagger.app/) does with deployments (please note that flagger does not support statefulsets at the moment, see the [issue](https://github.com/fluxcd/flagger/issues/410)).

### Requirements

A statefulset can be managed only if the `updateStrategy` is of type `RollingUpdate` (i.e. _not_ `OnDelete`), the operator will overwrite the statefulset `statefulset.spec.updateStrategy.rollingUpdate.partition` field so this must not be managed by other operators (e.g. flux). 

### Inner Workings

The rollout is controlled using the `statefulset.spec.updateStrategy.rollingUpdate.partition` field. When there is no need for a rolling update, the partition value is set to the number of replicas of the statefulset, to prevent uncontrolled rollouts. When the statefulset revision is updated and there is a need to perform a rolling update, for each pod to roll the phased rollout procedure will:
* ensure the previously rolled pod has been updated to the last revision of the statefulset;
* ensure all pods in the statefulset are available;
* wait a certain time to allow prometheus to gather fresh data;
* perform prometheus checks, at intervals: after a number of consecutive succesful checks, the rollout is considered safe to continue;
* if the rollout is considered safe to continue, decrease `statefulset.spec.updateStrategy.rollingUpdate.partition` to allow the normal rolling update procedure to continue.

Even if `statefulset.spec.updateStrategy.rollingUpdate.maxUnavailable` is respected, pods will be rolled one at a time. The normal rolling update checks are not superseded by the phased rollout checks (i.e. `statefulset.spec.minReadySeconds` and readiness probes will be respected).

Prometheus checks semantic is similar to prometheus alerts: if data is returned then the check is considered failed, if no data is returned the check is considered successful.

### Resource Example

This is an example of `PhasedRollout` resource:

```yaml
apiVersion: sts.plus/v1alpha1
kind: PhasedRollout
metadata:
  name: phasedrollout-sample
spec:
  targetRef: web               # the name of the statefulset to manage
  standardRollingUpdate: false # if true, the normal rolling update will be resumed
  check:
    initialDelaySeconds: 30 # initial delay to wait after a pod is rolled, to permit prometheus to get fresh data
    periodSeconds: 30       # interval between checks
    successThreshold: 3     # number of consecutive success checks to consider the rollout step good
    query:
      expr: "up{}"           # prometheus query 
      url: prom:9000/xx      # url of the prometheus endpoint (i.e. without the `/api/v1/query` path)
      secretRef: prom-secret # optional secret to authenticate to prometheus
--
apiVersion: v1
stringData:
  token: token       # optional token for bearer authentication
  password: password # optional username to use for basic authentication
  username: username # optional password to use for basic authentication
kind: Secret
metadata:
  name: prom-secret
type: Opaque

```

See the complete [Custom Resource Definition](./config/crd/bases/sts.plus_phasedrollouts.yaml).

### Step by Step Example

Here we describe a step by step example to test the operator.

1. Deploy the requirements for tests in the cluster, i.e. cert-manager and the prometheus-operator:
```sh
make deploy-test-requirements
```

2. Install the CRDs into the cluster:
```sh
make install
```

3. Deploy the controller to the cluster:
```sh
make deploy IMG=dbonfigli/sts-phased-rollout-operator:latest
```

4. Deploy the samples:
```sh
kubectl apply -k config/samples/
```

5. Trigger a change in the statefulset and watch the status of the phased rolling update:
```sh
kubectl patch sts web -n sample --patch '{"spec":{"template":{"metadata":{"labels":{"trigger-rollout":"'$(date +%s)'"}}}}}'
kubectl get phasedrollout phasedrollout-sample -o yaml -w -n sample
```

## Development

See the [development doc](./development.md) for instructions on how to test and build the operator.

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
