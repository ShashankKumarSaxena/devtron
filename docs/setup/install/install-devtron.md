# Install Devtron

Are you installing Devtron on Minikube, Microk8s, K3s, Kind? See Instructions [here](./Install-devtron-on-Minikube-Microk8s-K3s-Kind.md)

This page helps you to install Devtron without any integrations. Integrations can be added later using [Devtron Stack Manager](./../../user-guide/stack-manager.md).

## Before you begin

Install [Helm](https://helm.sh/docs/intro/install/) if you haven't done that already!

{% tabs %}
{% tab title="Install with default configurations" %}
```bash
helm repo add devtron https://helm.devtron.ai

helm install devtron devtron/devtron-operator --create-namespace --namespace devtroncd

```
{% endtab %}
{% endtabs %}

> To install Devtron on clusters with the multi-architecture nodes (ARM and AMD), append the installation command with `--set installer.arch=multi-arch`.

[//]: # (If you are planning to use Hyperion for `production deployments`, please refer to our recommended overrides for [Devtron Installation]&#40;override-default-devtron-installation-configs.md&#41;.)

[//]: # (## Installation status)

[//]: # ()
[//]: # (Run following command)

[//]: # ()
[//]: # (```bash)

[//]: # (kubectl -n devtroncd get installers installer-devtron -o jsonpath='{.status.sync.status}')

[//]: # (```)

## Devtron dashboard

Use the following command to get the dashboard URL:

```text
kubectl get svc -n devtroncd devtron-service -o jsonpath='{.status.loadBalancer.ingress}'
```

You will get the result something as shown below:

```text
[test2@server ~]$ kubectl get svc -n devtroncd devtron-service -o jsonpath='{.status.loadBalancer.ingress}'
[map[hostname:aaff16e9760594a92afa0140dbfd99f7-305259315.us-east-1.elb.amazonaws.com]]
```

The hostname `aaff16e9760594a92afa0140dbfd99f7-305259315.us-east-1.elb.amazonaws.com` as mentioned above is the Loadbalancer URL where you can access the Devtron dashboard.

> You can also do a CNAME entry corresponding to your domain/subdomain to point to this Loadbalancer URL to access it at a custom domain.

| Host | Type | Points to |
| :--- | :--- | :--- |
| devtron.yourdomain.com | CNAME | aaff16e9760594a92afa0140dbfd99f7-305259315.us-east-1.elb.amazonaws.com |

### Devtron Admin credentials

#### For Devtron version v0.6.0 and higher

Use username:`admin` and for password run command mentioned below.
```bash
kubectl -n devtroncd get secret devtron-secret -o jsonpath='{.data.ADMIN_PASSWORD}' | base64 -d
```

#### For Devtron version less than v0.6.0

Use username:`admin` and for password run command mentioned below.
```bash
kubectl -n devtroncd get secret devtron-secret -o jsonpath='{.data.ACD_PASSWORD}' | base64 -d
```

### Cleaning Helm installer

Please make sure that you do not have anything inside namespaces devtroncd, devtron-cd devtron-ci, and devtron-demo as the below steps will clean everything inside these namespaces
```
helm uninstall devtron --namespace devtroncd

kubectl delete -n devtroncd \
-f https://raw.githubusercontent.com/devtron-labs/charts/main/charts/devtron/crds/crd-devtron.yaml

kubectl delete ns devtroncd
```

## Upgrade

To use the CI/CD capabilities with Devtron, users can Install the [CI/CD integration](install-devtron-with-cicd.md).
