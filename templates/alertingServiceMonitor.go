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
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var params = map[string][]string{
	"match[]": {
		"ALERTS{alertstate='firing'}",
	},
}

var AlertingServiceMonitorTemplate = promv1.ServiceMonitor{
	Spec: promv1.ServiceMonitorSpec{
		Endpoints: []promv1.Endpoint{
			{
				Port:     "web",
				Path:     "/federate",
				Scheme:   "http",
				Interval: "10s",
				Params:   params,
			},
		},
		Selector: metav1.LabelSelector{
			MatchLabels: map[string]string{
				"operated-prometheus": "true",
			},
		},
	},
}
