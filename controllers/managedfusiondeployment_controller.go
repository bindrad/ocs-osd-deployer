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

package controllers

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/go-logr/logr"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	promv1a1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/red-hat-storage/ocs-osd-deployer/api/v1alpha1"
	"github.com/red-hat-storage/ocs-osd-deployer/templates"
	"github.com/red-hat-storage/ocs-osd-deployer/utils"

	opv1a1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
)

const (
	managedFusionDeploymentFinalizer  = "managedfusiondeployment.ocs.openshift.io"
	prometheusCatalogSourceName       = "prometheus-operator-source"
	prometheusSubscriptionName        = "downstream-prometheus-operator"
	prometheusName                    = "managed-fusion-prometheus"
	alertmanagerName                  = "managed-fusion-alertmanager"
	alertmanagerConfigName            = "managed-fusion-alertmanager-config"
	managedFusionDeploymentSecretName = "managed-fusion-config"
	managedFusionDeploymentCRName     = "managedfusion"
	monLabelKey                       = "app"
	monLabelValue                     = "managed-fusion"
	deployerCSVPrefix                 = "ocs-osd-deployer"
)

// ManagedFusionDeploymentReconciler reconciles a ManagedFusionDeployment object
type ManagedFusionDeploymentReconciler struct {
	client.Client
	UnrestrictedClient client.Client
	Log                logr.Logger
	Scheme             *runtime.Scheme

	ctx                            context.Context
	namespace                      string
	managedFusionDeployment        *v1alpha1.ManagedFusionDeployment
	prometheusOperatorSubscription *opv1a1.Subscription
	prometheusOperatorSource       *opv1a1.CatalogSource
	prometheus                     *promv1.Prometheus
	alertmanager                   *promv1.Alertmanager
	alertmanagerConfig             *promv1a1.AlertmanagerConfig
	managedFusionDeploymentSecret  *corev1.Secret
	CustomerNotificationHTMLPath   string
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedFusionDeploymentReconciler) SetupWithManager(mgr ctrl.Manager, ctrlOptions *controller.Options) error {
	if ctrlOptions == nil {
		ctrlOptions = &controller.Options{
			MaxConcurrentReconciles: 1,
		}
	}
	managedFusionDeploymentPredicates := builder.WithPredicates(
		predicate.GenerationChangedPredicate{},
	)
	monStatefulSetPredicates := builder.WithPredicates(
		predicate.NewPredicateFuncs(
			func(client client.Object) bool {
				name := client.GetName()
				return name == fmt.Sprintf("prometheus-%s", prometheusName) ||
					name == fmt.Sprintf("alertmanager-%s", alertmanagerName)
			},
		),
	)
	secretPredicates := builder.WithPredicates(
		predicate.NewPredicateFuncs(
			func(client client.Object) bool {
				name := client.GetName()
				return name == managedFusionDeploymentSecretName
			},
		),
	)
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(*ctrlOptions).
		For(&v1alpha1.ManagedFusionDeployment{}, managedFusionDeploymentPredicates).
		Owns(&opv1a1.CatalogSource{}).
		Owns(&opv1a1.Subscription{}).
		Owns(&promv1.Prometheus{}).
		Owns(&promv1.Alertmanager{}).
		Owns(&promv1a1.AlertmanagerConfig{}).
		Watches(
			&source.Kind{Type: &appsv1.StatefulSet{}},
			&handler.EnqueueRequestForObject{},
			monStatefulSetPredicates,
		).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			&handler.EnqueueRequestForObject{},
			secretPredicates,
		).
		Complete(r)
}

//+kubebuilder:rbac:groups=ocs.openshift.io,resources=managedfusiondeployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ocs.openshift.io,resources=managedfusiondeployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operators.coreos.com,namespace=system,resources=catalogsources,verbs=get;list;watch;delete;update;patch
//+kubebuilder:rbac:groups=operators.coreos.com,namespace=system,resources=subscriptions,verbs=get;list;watch;delete;update;patch
//+kubebuilder:rbac:groups=operators.coreos.com,namespace=system,resources=installplans,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=operators.coreos.com,namespace=system,resources=clusterserviceversions,verbs=get;list;watch;delete;update;patch
//+kubebuilder:rbac:groups="monitoring.coreos.com",namespace=system,resources={alertmanagers,prometheuses,alertmanagerconfigs},verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",namespace=system,resources=secrets,verbs=create;get;list;watch;update
//+kubebuilder:rbac:groups="apps",namespace=system,resources=statefulsets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ManagedFusionDeployment object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.4/pkg/reconcile
func (r *ManagedFusionDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("managedfusiondeployment", req.NamespacedName)
	log.Info("Starting reconcile for ManagedFusionDeployment")

	// TODO(user): your logic here
	r.initReconcile(ctx, req)

	if err := r.get(r.managedFusionDeployment); err != nil {
		if errors.IsNotFound(err) {
			r.Log.V(-1).Info("ManagedFusionDeployment resource not found")
			return ctrl.Result{}, nil
		}
		r.Log.Error(err, "Failed to get ManagedFusionDeployment")
		return ctrl.Result{}, fmt.Errorf("failed to get ManagedFusionDeployment: %v", err)
	}

	// Run the reconcile phases
	result, err := r.reconcilePhases()
	if err != nil {
		r.Log.Error(err, "An error was encountered during reconcilePhases")
	}

	// Ensure status is updated once even on failed reconciles
	var statusErr error
	if r.managedFusionDeployment.UID != "" {
		statusErr = r.Client.Status().Update(r.ctx, r.managedFusionDeployment)
	}
	if err != nil {
		return ctrl.Result{}, err
	} else if statusErr != nil {
		return ctrl.Result{}, statusErr
	} else {
		return result, nil
	}
}

func (r *ManagedFusionDeploymentReconciler) initReconcile(ctx context.Context, req ctrl.Request) {
	r.ctx = ctx
	r.namespace = req.Namespace

	r.managedFusionDeployment = &v1alpha1.ManagedFusionDeployment{}
	r.managedFusionDeployment.Name = managedFusionDeploymentCRName
	r.managedFusionDeployment.Namespace = r.namespace

	r.prometheusOperatorSource = &opv1a1.CatalogSource{}
	r.prometheusOperatorSource.Name = prometheusCatalogSourceName
	r.prometheusOperatorSource.Namespace = r.namespace

	r.prometheusOperatorSubscription = &opv1a1.Subscription{}
	r.prometheusOperatorSubscription.Name = prometheusSubscriptionName
	r.prometheusOperatorSubscription.Namespace = r.namespace

	r.prometheus = &promv1.Prometheus{}
	r.prometheus.Name = prometheusName
	r.prometheus.Namespace = r.namespace

	r.alertmanager = &promv1.Alertmanager{}
	r.alertmanager.Name = alertmanagerName
	r.alertmanager.Namespace = r.namespace

	r.alertmanagerConfig = &promv1a1.AlertmanagerConfig{}
	r.alertmanagerConfig.Name = alertmanagerConfigName
	r.alertmanagerConfig.Namespace = r.namespace

	r.managedFusionDeploymentSecret = &corev1.Secret{}
	r.managedFusionDeploymentSecret.Name = managedFusionDeploymentSecretName
	r.managedFusionDeploymentSecret.Namespace = r.namespace
}

func (r *ManagedFusionDeploymentReconciler) reconcilePhases() (reconcile.Result, error) {

	r.updateComponentStatus()

	if !r.managedFusionDeployment.DeletionTimestamp.IsZero() {
		if !r.areComponentsReadyForUninstall() {
			r.Log.Info("Sub-components are not in ready state, cannot proceed with uninstallation")
			return ctrl.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
		}

		found, err := r.doesManagedFusionOfferingExist()
		if err != nil {
			return ctrl.Result{}, err
		}
		if found {
			r.Log.Info("Found ManagedFusionOffering resource, cannot proceed with uninstallation")
			return ctrl.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
		}

		r.Log.Info("removing finalizer from the ManagedFusionDeployment resource")
		r.managedFusionDeployment.SetFinalizers(utils.Remove(r.managedFusionDeployment.GetFinalizers(),
			managedFusionDeploymentFinalizer))
		if err := r.Client.Update(r.ctx, r.managedFusionDeployment); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer from ManagedFusionDeployment: %v", err)
		}
		r.Log.Info("finallizer removed successfully")

		if err := r.removeOLMComponents(); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove agent CSV: %v", err)
		}
	} else if r.managedFusionDeployment.UID != "" {
		if !utils.Contains(r.managedFusionDeployment.GetFinalizers(), managedFusionDeploymentFinalizer) {
			r.Log.V(-1).Info("finalizer missing on the managedOCS resource, adding...")
			r.managedFusionDeployment.SetFinalizers(append(r.managedFusionDeployment.GetFinalizers(),
				managedFusionDeploymentFinalizer))
			if err := r.update(r.managedFusionDeployment); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update managedOCS with finalizer: %v", err)
			}
		}
		if err := r.reconcilePrometheusOperatorSource(); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.reconcilePrometheusOperatorSubscription(); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.reconcilePrometheusOperatorInstallPlan(); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.reconcilePrometheus(); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.reconcileAlertmanager(); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.reconcileAlertmanagerConfig(); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *ManagedFusionDeploymentReconciler) reconcilePrometheusOperatorSource() error {
	r.Log.Info("Reconciling Prometheus Operator CatalogSource")
	_, err := ctrl.CreateOrUpdate(r.ctx, r.Client, r.prometheusOperatorSource, func() error {
		if err := r.own(r.prometheusOperatorSource); err != nil {
			return err
		}
		desired := templates.PrometheusSource.DeepCopy()
		r.prometheusOperatorSource.Spec = desired.Spec
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (r *ManagedFusionDeploymentReconciler) reconcilePrometheusOperatorSubscription() error {
	r.Log.Info("Reconciling Prometheus Operator Subscription")
	_, err := ctrl.CreateOrUpdate(r.ctx, r.Client, r.prometheusOperatorSubscription, func() error {
		if err := r.own(r.prometheusOperatorSubscription); err != nil {
			return err
		}
		desired := templates.PrometheusSubscriptionTemplate.DeepCopy()
		desired.Spec.CatalogSource = prometheusCatalogSourceName
		desired.Spec.CatalogSourceNamespace = r.prometheusOperatorSource.Namespace
		r.prometheusOperatorSubscription.Spec = desired.Spec
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (r *ManagedFusionDeploymentReconciler) reconcilePrometheusOperatorInstallPlan() error {
	r.Log.Info("Reconciling Prometheus Operator InstallPlan")
	prometheusOperatorCSV := &opv1a1.ClusterServiceVersion{}
	prometheusOperatorCSV.Name = templates.PrometheusCSVName
	prometheusOperatorCSV.Namespace = r.namespace
	if err := r.get(prometheusOperatorCSV); err != nil {
		if errors.IsNotFound(err) {
			var foundInstallPlan bool
			installPlans := &opv1a1.InstallPlanList{}
			if err := r.list(installPlans); err != nil {
				return err
			}
			for i, installPlan := range installPlans.Items {
				if findInSlice(installPlan.Spec.ClusterServiceVersionNames, prometheusOperatorCSV.Name) {
					foundInstallPlan = true
					if installPlan.Spec.Approval == opv1a1.ApprovalManual &&
						!installPlan.Spec.Approved {
						installPlans.Items[i].Spec.Approved = true
						if err := r.update(&installPlans.Items[i]); err != nil {
							return err
						}
					}
				}
			}
			if !foundInstallPlan {
				return fmt.Errorf("installPlan not found for CSV %s", prometheusOperatorCSV.Name)
			}
		}
		return fmt.Errorf("failed to get Prometheus Operator CSV: %v", err)
	}
	return nil
}

func (r *ManagedFusionDeploymentReconciler) reconcilePrometheus() error {
	r.Log.Info("Reconciling Prometheus")

	_, err := ctrl.CreateOrUpdate(r.ctx, r.Client, r.prometheus, func() error {
		if err := r.own(r.prometheus); err != nil {
			return err
		}

		desired := templates.PrometheusTemplate.DeepCopy()
		utils.AddLabel(r.prometheus, monLabelKey, monLabelValue)

		r.prometheus.Spec = desired.Spec
		r.prometheus.Spec.Alerting.Alertmanagers[0].Namespace = r.namespace

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to update Prometheus: %v", err)
	}

	return nil
}

func (r *ManagedFusionDeploymentReconciler) reconcileAlertmanager() error {
	r.Log.Info("Reconciling Alertmanager")
	_, err := ctrl.CreateOrUpdate(r.ctx, r.Client, r.alertmanager, func() error {
		if err := r.own(r.alertmanager); err != nil {
			return err
		}

		desired := templates.AlertmanagerTemplate.DeepCopy()
		desired.Spec.AlertmanagerConfigSelector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				monLabelKey: monLabelValue,
			},
		}
		r.alertmanager.Spec = desired.Spec
		utils.AddLabel(r.alertmanager, monLabelKey, monLabelValue)

		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (r *ManagedFusionDeploymentReconciler) reconcileAlertmanagerConfig() error {
	r.Log.Info("Reconciling AlertmanagerConfig")

	_, err := ctrl.CreateOrUpdate(r.ctx, r.Client, r.alertmanagerConfig, func() error {
		if err := r.own(r.alertmanagerConfig); err != nil {
			return err
		}

		managedFusionDeploymentSpec := r.managedFusionDeployment.Spec
		if managedFusionDeploymentSpec.Pager.SOPEndpoint == "" {
			return fmt.Errorf("managedFusionDeployment CR does not contain a SOPEndpoint entry")
		}

		alertingAddressList := []string{}
		alertingAddressList = append(alertingAddressList,
			managedFusionDeploymentSpec.SMTP.NotificationEmails...)
		if managedFusionDeploymentSpec.SMTP.Username == "" {
			return fmt.Errorf("managedFusionDeployment CR does not contain a username entry")
		}
		if managedFusionDeploymentSpec.SMTP.Endpoint == "" {
			return fmt.Errorf("managedFusionDeployment CR does not contain a endpoint entry")
		}
		if managedFusionDeploymentSpec.SMTP.FromAddress == "" {
			return fmt.Errorf("managedFusionDeployment CR does not contain a fromAddress entry")
		}
		smtpHTML, err := ioutil.ReadFile(r.CustomerNotificationHTMLPath)
		if err != nil {
			return fmt.Errorf("unable to read customernotification.html file: %v", err)
		}

		// if r.managedFusionDeploymentSecret.UID == "" {
		if err := r.get(r.managedFusionDeploymentSecret); err != nil {
			return fmt.Errorf("unable to get managed-fusion-config secret: %v", err)
		}
		// }
		managedFusionDeploymentSecretData := r.managedFusionDeploymentSecret.Data
		pagerdutyServiceKey, found := managedFusionDeploymentSecretData["pagerKey"]
		if !found {
			return fmt.Errorf("managedFusionDeploymentSecret does not contain a pagerKey entry")
		} else {
			if string(pagerdutyServiceKey) == "" {
				return fmt.Errorf("managedFusionDeploymentSecret contains an empty pagerKey entry")
			}
		}

		smtpPassword, found := managedFusionDeploymentSecretData["smtpPassword"]
		if !found {
			return fmt.Errorf("managedFusionDeploymentSecret does not contain a smtpPassword entry")
		} else {
			if string(smtpPassword) == "" {
				return fmt.Errorf("managedFusionDeploymentSecret contains an empty smtpPassword entry")
			}
		}

		desired := templates.AlertmanagerConfigTemplate.DeepCopy()
		for i := range desired.Spec.Receivers {
			receiver := &desired.Spec.Receivers[i]
			switch receiver.Name {
			case "pagerduty":
				receiver.PagerDutyConfigs[0].ServiceKey.Key = "pagerKey"
				receiver.PagerDutyConfigs[0].ServiceKey.LocalObjectReference.Name = r.managedFusionDeploymentSecret.Name
				receiver.PagerDutyConfigs[0].Details[0].Key = "SOP"
				receiver.PagerDutyConfigs[0].Details[0].Value = managedFusionDeploymentSpec.Pager.SOPEndpoint
			case "SendGrid":
				if len(alertingAddressList) > 0 {
					receiver.EmailConfigs[0].Smarthost = managedFusionDeploymentSpec.SMTP.Endpoint
					receiver.EmailConfigs[0].AuthUsername = managedFusionDeploymentSpec.SMTP.Username
					receiver.EmailConfigs[0].AuthPassword.LocalObjectReference.Name = r.managedFusionDeploymentSecret.Name
					receiver.EmailConfigs[0].AuthPassword.Key = "smtpPassword"
					receiver.EmailConfigs[0].From = managedFusionDeploymentSpec.SMTP.FromAddress
					receiver.EmailConfigs[0].To = strings.Join(alertingAddressList, ", ")
					receiver.EmailConfigs[0].HTML = string(smtpHTML)
				} else {
					r.Log.V(-1).Info("Customer Email for alert notification is not provided")
					receiver.EmailConfigs = []promv1a1.EmailConfig{}
				}
			}
		}
		r.alertmanagerConfig.Spec = desired.Spec
		utils.AddLabel(r.alertmanagerConfig, monLabelKey, monLabelValue)

		return nil
	})

	return err
}

func (r *ManagedFusionDeploymentReconciler) updateComponentStatus() {
	// Getting the status of the Prometheus component.
	promStatus := &r.managedFusionDeployment.Status.Components.Prometheus
	if err := r.get(r.prometheus); err == nil {
		promStatefulSet := &appsv1.StatefulSet{}
		promStatefulSet.Namespace = r.namespace
		promStatefulSet.Name = fmt.Sprintf("prometheus-%s", prometheusName)
		if err := r.get(promStatefulSet); err == nil {
			desiredReplicas := int32(1)
			if r.prometheus.Spec.Replicas != nil {
				desiredReplicas = *r.prometheus.Spec.Replicas
			}
			if promStatefulSet.Status.ReadyReplicas != desiredReplicas {
				promStatus.State = v1alpha1.ComponentPending
			} else {
				promStatus.State = v1alpha1.ComponentReady
			}
		} else {
			promStatus.State = v1alpha1.ComponentPending
		}
	} else if errors.IsNotFound(err) {
		promStatus.State = v1alpha1.ComponentNotFound
	} else {
		r.Log.V(-1).Info("error getting Prometheus, setting compoment status to Unknown")
		promStatus.State = v1alpha1.ComponentUnknown
	}

	// Getting the status of the Alertmanager component.
	amStatus := &r.managedFusionDeployment.Status.Components.Alertmanager
	if err := r.get(r.alertmanager); err == nil {
		amStatefulSet := &appsv1.StatefulSet{}
		amStatefulSet.Namespace = r.namespace
		amStatefulSet.Name = fmt.Sprintf("alertmanager-%s", alertmanagerName)
		if err := r.get(amStatefulSet); err == nil {
			desiredReplicas := int32(1)
			if r.alertmanager.Spec.Replicas != nil {
				desiredReplicas = *r.alertmanager.Spec.Replicas
			}
			if amStatefulSet.Status.ReadyReplicas != desiredReplicas {
				amStatus.State = v1alpha1.ComponentPending
			} else {
				amStatus.State = v1alpha1.ComponentReady
			}
		} else {
			amStatus.State = v1alpha1.ComponentPending
		}
	} else if errors.IsNotFound(err) {
		amStatus.State = v1alpha1.ComponentNotFound
	} else {
		r.Log.V(-1).Info("error getting Alertmanager, setting compoment status to Unknown")
		amStatus.State = v1alpha1.ComponentUnknown
	}
}

func (r *ManagedFusionDeploymentReconciler) doesManagedFusionOfferingExist() (bool, error) {
	managedFusionOfferingList := v1alpha1.ManagedFusionOfferingList{}
	if err := r.UnrestrictedClient.List(r.ctx, &managedFusionOfferingList); err != nil {
		return false, fmt.Errorf("unable to list managedFusionOffering CR: %v", err)
	}
	if len(managedFusionOfferingList.Items) > 0 {
		return true, nil
	}
	return false, nil
}

func (r *ManagedFusionDeploymentReconciler) areComponentsReadyForUninstall() bool {
	subComponents := r.managedFusionDeployment.Status.Components
	return subComponents.Prometheus.State == v1alpha1.ComponentReady &&
		subComponents.Alertmanager.State == v1alpha1.ComponentReady
}

func (r *ManagedFusionDeploymentReconciler) removeOLMComponents() error {

	r.Log.Info("deleting deployer csv")
	if err := r.deleteCSVByPrefix(deployerCSVPrefix); err != nil {
		return fmt.Errorf("Unable to delete csv: %v", err)
	} else {
		r.Log.Info("Deployer csv removed successfully")
		return nil
	}
}

func (r *ManagedFusionDeploymentReconciler) deleteCSVByPrefix(name string) error {
	if csv, err := r.getCSVByPrefix(name); err == nil {
		return r.delete(csv)
	} else if errors.IsNotFound(err) {
		return nil
	} else {
		return err
	}
}

func (r *ManagedFusionDeploymentReconciler) getCSVByPrefix(name string) (*opv1a1.ClusterServiceVersion, error) {
	csvList := opv1a1.ClusterServiceVersionList{}
	if err := r.list(&csvList); err != nil {
		return nil, fmt.Errorf("unable to list csv resources: %v", err)
	}

	for index := range csvList.Items {
		candidate := &csvList.Items[index]
		if strings.HasPrefix(candidate.Name, name) {
			return candidate, nil
		}
	}
	return nil, errors.NewNotFound(opv1a1.Resource("csv"), fmt.Sprintf("unable to find a csv prefixed with %s", name))
}

func (r *ManagedFusionDeploymentReconciler) get(obj client.Object) error {
	key := client.ObjectKeyFromObject(obj)
	return r.Client.Get(r.ctx, key, obj)
}

func (r *ManagedFusionDeploymentReconciler) list(obj client.ObjectList) error {
	listOptions := client.InNamespace(r.namespace)
	return r.Client.List(r.ctx, obj, listOptions)
}

func (r *ManagedFusionDeploymentReconciler) update(obj client.Object) error {
	return r.Client.Update(r.ctx, obj)
}

func (r *ManagedFusionDeploymentReconciler) delete(obj client.Object) error {
	if err := r.Client.Delete(r.ctx, obj); err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *ManagedFusionDeploymentReconciler) own(resource metav1.Object) error {
	if err := ctrl.SetControllerReference(r.managedFusionDeployment, resource, r.Scheme); err != nil {
		return err
	}
	return nil
}

func findInSlice(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
