/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package templates

import (
	promv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var params = map[string][]string{
	"match[]": {
		"{__name__='kube_node_status_condition'}",
		"{__name__='kube_persistentvolume_info'}",
		"{__name__='kube_storageclass_info'}",
		"{__name__='kube_persistentvolumeclaim_info'}",
		"{__name__='kubelet_volume_stats_capacity_bytes'}",
		"{__name__='kubelet_volume_stats_used_bytes'}",
		"{__name__='node_disk_read_time_seconds_total'}",
		"{__name__='node_disk_write_time_seconds_total'}",
		"{__name__='node_disk_reads_completed_total'}",
		"{__name__='node_disk_writes_completed_total'}",
	},
}

var ServiceMonitorTemplate = promv1.ServiceMonitor{
	Spec: promv1.ServiceMonitorSpec{
		Endpoints: []promv1.Endpoint{
			{
				Port:          "web",
				Path:          "/federate",
				Scheme:        "https",
				ScrapeTimeout: "30s",
				Interval:      "30s",
				HonorLabels:   true,
				TLSConfig: &promv1.TLSConfig{
					InsecureSkipVerify: true,
				},
				Params: params,
			},
		},
		NamespaceSelector: promv1.NamespaceSelector{
			MatchNames: []string{"openshift-monitoring"},
		},
		Selector: metav1.LabelSelector{
			MatchLabels: map[string]string{
				"prometheus": "k8s",
			},
		},
	},
}
