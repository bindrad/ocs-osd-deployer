package templates

import (
	"time"

	opv1a1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var PrometheusSource = opv1a1.CatalogSource{
	Spec: opv1a1.CatalogSourceSpec{
		SourceType: opv1a1.SourceTypeGrpc,
		Image:      "quay.io/dbindra/ose-prometheus-operator:4.10.0_catsrc",
		UpdateStrategy: &opv1a1.UpdateStrategy{
			RegistryPoll: &opv1a1.RegistryPoll{
				Interval: &v1.Duration{
					Duration: time.Minute * 30,
				},
			},
		},
		DisplayName: "downstream-prometheus-operator",
		Publisher:   "Red Hat",
	},
}
