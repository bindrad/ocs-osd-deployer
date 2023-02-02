package templates

import opv1a1 "github.com/operator-framework/api/pkg/operators/v1alpha1"

var OCSSubscription = opv1a1.Subscription{
	Spec: &opv1a1.SubscriptionSpec{
		Package:             "ocs-operator",
		InstallPlanApproval: opv1a1.ApprovalManual,
	},
}
