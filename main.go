package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type Config struct {
	KubeconfigPath  string `yaml:"kubeconfig_path"`
	NamespacePrefix string `yaml:"namespace_prefix"`
	LokiAddress     string `yaml:"loki_address"`
}

type LokiQueryRangeRequest struct {
	Query string `json:"query"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Step  int    `json:"step"`
}

type LokiQueryRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]interface{}   `json:"values"`
		} `json:"result"`
		Stats struct{} `json:"stats"`
	} `json:"data"`
}

type MismatchRecord struct {
	PodName       string
	Namespace     string
	TotalLogLines int
	LogLineCount  int
}

func getLogLineCount(podStartTime time.Time, podName string, namespace string, lokiAddress string) (int, error) {
	startedAt := podStartTime.UnixNano()
	end := podStartTime.Add(time.Hour).UnixNano() // 1 hour after pod start

	query := fmt.Sprintf(`count_over_time({pod_name="%s"}[1h])`, podName)

	u, err := url.Parse(fmt.Sprintf("%s/loki/api/v1/query_range", lokiAddress))
	if err != nil {
		return 0, err
	}

	params := url.Values{}
	params.Set("query", query)
	params.Set("start", strconv.FormatInt(startedAt, 10))
	params.Set("end", strconv.FormatInt(end, 10))
	params.Set("step", "1h")
	u.RawQuery = params.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("X-Scope-OrgID", namespace)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("Loki query failed with status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var lokiResp LokiQueryRangeResponse
	err = json.Unmarshal(body, &lokiResp)
	if err != nil {
		return 0, err
	}

	if len(lokiResp.Data.Result) > 0 {
		result, err := strconv.Atoi(lokiResp.Data.Result[0].Values[0][1].(string))
		if err != nil {
			return 0, err
		}

		return result, nil
	}

	return 0, fmt.Errorf("no logs found for pod %s", podName)
}

func main() {
	configFile := "config.yaml"

	configFileData, err := os.Open(configFile)
	if err != nil {
		log.Fatalf("Failed to open config file: %v", err)
	}
	defer configFileData.Close()

	var config Config
	decoder := yaml.NewDecoder(configFileData)
	err = decoder.Decode(&config)
	if err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}

	if config.NamespacePrefix == "" {
		config.NamespacePrefix = "logger-ns"
	}

	if config.KubeconfigPath == "" {
		config.KubeconfigPath = filepath.Join(homedir.HomeDir(), ".kube", "config")
	}

	kubeconfig, err := clientcmd.BuildConfigFromFlags("", config.KubeconfigPath)
	if err != nil {
		log.Fatalf("Error building kubeconfig from %s: %v", config.KubeconfigPath, err)
	}

	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		log.Fatalf("Error creating Kubernetes client: %v", err)
	}

	namespaces, err := clientset.CoreV1().Namespaces().List(context.TODO(), v1.ListOptions{})
	if err != nil {
		log.Fatalf("Failed to list namespaces: %v", err)
	}

	var mismatchRecords []MismatchRecord

	for _, namespace := range namespaces.Items {
		if !isTargetNamespace(namespace.Name, config.NamespacePrefix) {
			continue
		}

		pods, err := clientset.CoreV1().Pods(namespace.Name).List(context.TODO(), v1.ListOptions{})
		if err != nil {
			log.Fatalf("Failed to list pods in namespace %s: %v", namespace.Name, err)
		}

		for _, pod := range pods.Items {
			totalLogLines, err := strconv.Atoi(pod.Annotations["total_log_lines"])
			if err != nil {
				log.Printf("Failed to get total log lines for pod %s in namespace %s: %v", pod.Name, namespace.Name, err)
				continue
			}

			logLineCount, err := getLogLineCount(pod.Status.StartTime.Time, pod.Name, namespace.Name, config.LokiAddress)
			if err != nil {
				log.Printf("Failed to get log line count for pod %s in namespace %s: %v", pod.Name, namespace.Name, err)
				continue
			}

			if totalLogLines != logLineCount {
				mismatchRecord := MismatchRecord{
					PodName:       pod.Name,
					Namespace:     namespace.Name,
					TotalLogLines: totalLogLines,
					LogLineCount:  logLineCount,
				}
				mismatchRecords = append(mismatchRecords, mismatchRecord)
				log.Printf("Mismatch for pod %s in namespace %s: total_log_lines=%d, log_line_count=%d", pod.Name, namespace.Name, totalLogLines, logLineCount)
			} else {
				log.Printf("Match for pod %s in namespace %s: total_log_lines=%d, log_line_count=%d", pod.Name, namespace.Name, totalLogLines, logLineCount)
			}
		}
	}

	if len(mismatchRecords) > 0 {
		log.Println("Mismatch records:")
		for _, record := range mismatchRecords {
			fmt.Printf("Pod: %s, Namespace: %s, TotalLogLines: %d, LogLineCount: %d\n", record.PodName, record.Namespace, record.TotalLogLines, record.LogLineCount)
		}
	} else {
		log.Println("No mismatches found")
	}
}

func isTargetNamespace(namespaceName, namespacePrefix string) bool {
	return len(namespaceName) >= len(namespacePrefix) && namespaceName[:len(namespacePrefix)] == namespacePrefix
}
