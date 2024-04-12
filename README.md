# k8s-loki-logline-verifier

This Go program is designed to verify the consistency between the number of log lines reported in Kubernetes pod annotations and the actual number of log lines retrieved from Loki.

**Notice: This is a sample implementation of a program to check that the number of lines in the log output by k8s Pod matches the number of lines in the k8s Pod log stored in Loki.**

## Motivation

I wanted to check that the k8s Pod logs generated at [zinrai/k8s-pod-log-generator](https://github.com/zinrai/k8s-pod-log-generator) were stored in Loki without any missing logs.

## Tested Version

- `Loki`: 2.9.5
    - https://grafana.com/docs/loki/latest/setup/install/helm/install-scalable/
- `Promtail`: 2.9.3
    - https://grafana.com/docs/loki/latest/send-data/promtail/installation/#install-using-helm

## Requirements

- Annotations when [zinrai/k8s-pod-log-generator](https://github.com/zinrai/k8s-loki-logline-verifier) is executed Use the key and value of the number of rows in the attached log.
- Access to a Grafana Loki instance with `auth_enabled: true`
    - https://grafana.com/docs/loki/latest/configure/#supported-contents-and-default-values-of-lokiyaml
- Access to Loki search endpoints deployed on k8s is required.
- k8s Namespace is set as the unit of tenant id in Loki.

Example of access to a Loki search endpoint using port forwarding:

```
$ kubectl get service -n loki
NAME                        TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)             AGE
loki-backend                ClusterIP   10.24.124.202   <none>        3100/TCP,9095/TCP   20d
loki-backend-headless       ClusterIP   None            <none>        3100/TCP,9095/TCP   20d
loki-gateway                ClusterIP   10.24.120.118   <none>        80/TCP              20d
loki-memberlist             ClusterIP   None            <none>        7946/TCP            20d
loki-read                   ClusterIP   10.24.123.180   <none>        3100/TCP,9095/TCP   20d
loki-read-headless          ClusterIP   None            <none>        3100/TCP,9095/TCP   20d
loki-write                  ClusterIP   10.24.120.24    <none>        3100/TCP,9095/TCP   20d
loki-write-headless         ClusterIP   None            <none>        3100/TCP,9095/TCP   20d
query-scheduler-discovery   ClusterIP   None            <none>        3100/TCP,9095/TCP   20d
$
```
```
$ kubectl port-forward svc/loki-gateway 8080:80 -n loki
Forwarding from 127.0.0.1:8080 -> 8080
Forwarding from [::1]:8080 -> 8080
Handling connection for 8080
Handling connection for 8080
```

Example values.yaml for Promtail using Helm when logging from Cloud Pub/Sub:

```
daemonset:
  enabled: false
deployment:
  enabled: true
serviceAccount:
  create: false
  name: ksa-cloudpubsub
configmap:
  enabled: true
config:
  clients:
    - url: http://loki-gateway.loki.svc.cluster.local/loki/api/v1/push
      tenant_id: default
  snippets:
    scrapeConfigs: |
      - job_name: gcplog
        pipeline_stages:
          - tenant:
              label: "namespace"
        gcplog:
          subscription_type: "pull"
          project_id: "project-id"
          subscription: "subscription-id"
          use_incoming_timestamp: false
          use_full_line: false
          labels:
            job: "gcplog"
        relabel_configs:
          - source_labels: ['__gcp_resource_labels_namespace_name']
            target_label: 'namespace'
          - source_labels: ['__gcp_resource_labels_pod_name']
            target_label: 'pod_name'
```

## Configuration

The program reads configurations from a YAML file named `config.yaml`. The following configuration options are available:

- `kubeconfig_path`: (Optional) Path to the Kubernetes cluster configuration file. If not provided, the default path will be used.
- `namespace_prefix`: (Optional) Prefix for the namespaces created by the tool. Defaults to logger-ns.
- `loki_address`: URL of the Loki server.

## Usage

```bash
$ cat << EOF > config.yaml
loki_address: "http://localhost:8080"
EOF
```

```bash
$ go run main.go
2024/04/12 11:36:34 Match for pod logger-pod-13 in namespace logger-ns-1: total_log_lines=2560, log_line_count=2560
2024/04/12 11:36:34 Match for pod logger-pod-3 in namespace logger-ns-1: total_log_lines=2560, log_line_count=2560
2024/04/12 11:36:34 Match for pod logger-pod-1 in namespace logger-ns-2: total_log_lines=2560, log_line_count=2560
2024/04/12 11:36:34 Match for pod logger-pod-11 in namespace logger-ns-2: total_log_lines=2560, log_line_count=2560
2024/04/12 11:36:34 Match for pod logger-pod-12 in namespace logger-ns-3: total_log_lines=2560, log_line_count=2560
2024/04/12 11:36:35 Match for pod logger-pod-7 in namespace logger-ns-4: total_log_lines=2560, log_line_count=2560
2024/04/12 11:36:35 Match for pod logger-pod-10 in namespace logger-ns-5: total_log_lines=2560, log_line_count=2560
...
```

## License

This project is licensed under the MIT License - see the [LICENSE](https://opensource.org/license/mit) for details.
