package templates

import (
	"time"

	opv1a1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var OCSSource = opv1a1.CatalogSource{
	Spec: opv1a1.CatalogSourceSpec{
		SourceType: opv1a1.SourceTypeGrpc,
		UpdateStrategy: &opv1a1.UpdateStrategy{
			RegistryPoll: &opv1a1.RegistryPoll{
				Interval: &v1.Duration{
					Duration: time.Minute * 30,
				},
			},
		},
		DisplayName: "managed-fusion-ocs",
		Publisher:   "Red Hat",
	},
}
