package templates

import opv1a1 "github.com/operator-framework/api/pkg/operators/v1alpha1"

const PrometheusCSVName = "ose-prometheus-operator.4.10.0"

var PrometheusSubscriptionTemplate = opv1a1.Subscription{
	Spec: &opv1a1.SubscriptionSpec{
		Package:             "ose-prometheus-operator",
		InstallPlanApproval: opv1a1.ApprovalManual,
		Channel:             "beta",
		StartingCSV:         PrometheusCSVName,
	},
}
