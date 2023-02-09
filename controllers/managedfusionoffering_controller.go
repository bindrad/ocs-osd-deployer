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
	"math"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	opv1a1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	ocsv1 "github.com/red-hat-storage/ocs-operator/api/v1"
	"github.com/red-hat-storage/ocs-osd-deployer/api/v1alpha1"
	"github.com/red-hat-storage/ocs-osd-deployer/templates"
	"github.com/red-hat-storage/ocs-osd-deployer/utils"
)

const (
	ocsOperatorName   = "ocs-operator"
	rookConfigMapName = "rook-ceph-operator-config"
	mcgOperatorName   = "mcg-operator"
	storageSizeKey    = "size"
	deviceSetName     = "default"
)

// ManagedFusionOfferingReconciler reconciles a ManagedFusionOffering object
type ManagedFusionOfferingReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	ctx                   context.Context
	namespace             *corev1.Namespace
	managedFusionOffering *v1alpha1.ManagedFusionOffering
	OCSOperatorSource     *opv1a1.CatalogSource
	OCSSubscription       *opv1a1.Subscription
	storageCluster        *ocsv1.StorageCluster
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedFusionOfferingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	managedFusionOfferingPredicates := builder.WithPredicates(
		predicate.GenerationChangedPredicate{},
	)
	csvPredicates := builder.WithPredicates(
		predicate.NewPredicateFuncs(
			func(client client.Object) bool {
				return strings.HasPrefix(client.GetName(), ocsOperatorName)
			},
		),
	)
	configMapPredicates := builder.WithPredicates(
		predicate.NewPredicateFuncs(
			func(client client.Object) bool {
				name := client.GetName()
				return name == rookConfigMapName
			},
		),
	)
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ManagedFusionOffering{}, managedFusionOfferingPredicates).
		Owns(&opv1a1.CatalogSource{}).
		Owns(&opv1a1.Subscription{}).
		Owns(&ocsv1.StorageCluster{}).
		Watches(
			&source.Kind{Type: &opv1a1.ClusterServiceVersion{}},
			&handler.EnqueueRequestForObject{},
			csvPredicates,
		).
		Watches(
			&source.Kind{Type: &corev1.ConfigMap{}},
			&handler.EnqueueRequestForObject{},
			configMapPredicates,
		).
		Watches(
			&source.Kind{Type: &ocsv1.OCSInitialization{}},
			&handler.EnqueueRequestForObject{},
		).
		Complete(r)
}

//+kubebuilder:rbac:groups=ocs.openshift.io,resources=managedfusionofferings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ocs.openshift.io,resources=managedfusionofferings/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ManagedFusionOffering object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.4/pkg/reconcile
func (r *ManagedFusionOfferingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("managedfusionoffering", req.NamespacedName)

	// TODO(user): your logic here
	r.Log.Info(req.String())

	managedFusionOfferingList := &v1alpha1.ManagedFusionOfferingList{}
	if err := r.Client.List(ctx, managedFusionOfferingList); err != nil {
		return ctrl.Result{}, err
	}
	if len(managedFusionOfferingList.Items) == 0 {
		r.Log.Info("no managedFusionOffering CR found")
		return ctrl.Result{}, nil
	}

	for i := range managedFusionOfferingList.Items {
		r.managedFusionOffering = &managedFusionOfferingList.Items[i]
		switch r.managedFusionOffering.Spec.OfferingType {
		case "ODF":
			r.ctx = ctx
			r.namespace = &corev1.Namespace{}
			r.namespace.Name = req.Namespace

			// add operator group

			r.OCSOperatorSource = &opv1a1.CatalogSource{}
			r.OCSOperatorSource.Name = "ocs-catsrc"
			r.OCSOperatorSource.Namespace = r.namespace.Name

			r.OCSSubscription = &opv1a1.Subscription{}
			r.OCSSubscription.Name = "ocs-subs"
			r.OCSSubscription.Namespace = r.namespace.Name

			r.storageCluster = &ocsv1.StorageCluster{}
			r.storageCluster.Name = "ocs-storagecluster"
			r.storageCluster.Namespace = r.namespace.Name

			if err := r.reconcileODF(); err != nil {
				return ctrl.Result{}, err
			}
		// case "ODFClient":
		// 	r.reconcileODFClient()
		default:
			return ctrl.Result{}, fmt.Errorf("unknown Offering type")
		}
	}

	return ctrl.Result{}, nil
}

func (r *ManagedFusionOfferingReconciler) reconcileODF() error {
	if err := r.reconcileOCSSource(); err != nil {
		return err
	}

	if err := r.reconcileOCSSubscription(); err != nil {
		return err
	}

	if err := r.reconcileOCSOperatorInstallPlan(); err != nil {
		return err
	}

	if err := r.reconcileCSV(); err != nil {
		return err
	}

	if err := r.recocileProviderStorageCluster(); err != nil {
		return err
	}

	if err := r.reconcileRookCephOperatorConfig(); err != nil {
		return err
	}

	if err := r.reconcileOCSInitialization(); err != nil {
		return err
	}

	return nil
}

func (r *ManagedFusionOfferingReconciler) reconcileOCSSource() error {
	r.Log.Info("Reconciling Offering Source")
	_, err := ctrl.CreateOrUpdate(r.ctx, r.Client, r.OCSOperatorSource, func() error {
		if err := r.own(r.OCSOperatorSource); err != nil {
			return err
		}
		desired := templates.OCSSource.DeepCopy()
		switch r.managedFusionOffering.Spec.Release {
		case "4.10":
			desired.Spec.Image = "registry.redhat.io/redhat/redhat-operator-index:v4.10"
		case "4.11":
			desired.Spec.Image = "registry.redhat.io/redhat/redhat-operator-index:v4.11"
		}
		r.OCSOperatorSource.Spec = desired.Spec
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (r *ManagedFusionOfferingReconciler) reconcileOCSSubscription() error {
	r.Log.Info("Reconciling Offering Subscription")
	_, err := ctrl.CreateOrUpdate(r.ctx, r.Client, r.OCSSubscription, func() error {
		if err := r.own(r.OCSSubscription); err != nil {
			return err
		}
		desired := templates.OCSSubscription.DeepCopy()
		desired.Spec.CatalogSource = r.OCSOperatorSource.Name
		desired.Spec.CatalogSourceNamespace = r.OCSOperatorSource.Namespace
		switch r.managedFusionOffering.Spec.Release {
		case "4.10":
			desired.Spec.Channel = "stable-4.10"
			desired.Spec.StartingCSV = "ocs-operator.v4.10.8"
		case "4.11":
			desired.Spec.Channel = "stable-4.11"
			desired.Spec.StartingCSV = "ocs-operator.v4.11.2"
		}
		r.OCSSubscription.Spec = desired.Spec
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (r *ManagedFusionOfferingReconciler) reconcileOCSOperatorInstallPlan() error {
	r.Log.Info("Reconciling OCS Operator InstallPlan")
	OCSOperatorCSV := &opv1a1.ClusterServiceVersion{}
	OCSOperatorCSV.Name = r.OCSSubscription.Spec.StartingCSV
	OCSOperatorCSV.Namespace = r.OCSOperatorSource.Namespace
	if err := r.get(OCSOperatorCSV); err != nil {
		if apierrors.IsNotFound(err) {
			var foundInstallPlan bool
			installPlans := &opv1a1.InstallPlanList{}
			if err := r.list(installPlans); err != nil {
				return err
			}
			for i, installPlan := range installPlans.Items {
				if findInSlice(installPlan.Spec.ClusterServiceVersionNames, OCSOperatorCSV.Name) {
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
				return fmt.Errorf("installPlan not found for CSV %s", OCSOperatorCSV.Name)
			}
		}
		return fmt.Errorf("failed to get Prometheus Operator CSV: %v", err)
	}

	return nil
}

func (r *ManagedFusionOfferingReconciler) reconcileCSV() error {
	r.Log.Info("Reconciling CSVs")

	csvList := opv1a1.ClusterServiceVersionList{}
	if err := r.list(&csvList); err != nil {
		return fmt.Errorf("unable to list csv resources: %v", err)
	}

	for index := range csvList.Items {
		csv := &csvList.Items[index]
		if strings.HasPrefix(csv.Name, ocsOperatorName) {
			if err := r.updateOCSCSV(csv); err != nil {
				return fmt.Errorf("Failed to update OCS CSV: %v", err)
			}
		} else if strings.HasPrefix(csv.Name, mcgOperatorName) {
			if err := r.updateMCGCSV(csv); err != nil {
				return fmt.Errorf("Failed to update MCG CSV: %v", err)
			}
		}
	}
	return nil
}

func (r *ManagedFusionOfferingReconciler) updateOCSCSV(csv *opv1a1.ClusterServiceVersion) error {
	isChanged := false
	deployments := csv.Spec.InstallStrategy.StrategySpec.DeploymentSpecs
	for i := range deployments {
		containers := deployments[i].Spec.Template.Spec.Containers
		for j := range containers {
			switch container := &containers[j]; container.Name {
			case "ocs-operator":
				resources := utils.GetResourceRequirements("ocs-operator")
				if !equality.Semantic.DeepEqual(container.Resources, resources) {
					container.Resources = resources
					isChanged = true
				}
			case "rook-ceph-operator":
				resources := utils.GetResourceRequirements("rook-ceph-operator")
				if !equality.Semantic.DeepEqual(container.Resources, resources) {
					container.Resources = resources
					isChanged = true
				}
			case "ocs-metrics-exporter":
				resources := utils.GetResourceRequirements("ocs-metrics-exporter")
				if !equality.Semantic.DeepEqual(container.Resources, resources) {
					container.Resources = resources
					isChanged = true
				}
			default:
				r.Log.V(-1).Info("Could not find resource requirement", "Resource", container.Name)
			}
		}
	}
	if isChanged {
		if err := r.update(csv); err != nil {
			return fmt.Errorf("Failed to update OCS CSV: %v", err)
		}
	}
	return nil
}

func (r *ManagedFusionOfferingReconciler) updateMCGCSV(csv *opv1a1.ClusterServiceVersion) error {
	isChanged := false
	mcgDeployments := csv.Spec.InstallStrategy.StrategySpec.DeploymentSpecs
	for i := range mcgDeployments {
		deployment := &mcgDeployments[i]
		// Disable noobaa operator by scaling down the replica of noobaa deploymnet
		// in MCG Operator CSV.
		if deployment.Name == "noobaa-operator" &&
			(deployment.Spec.Replicas == nil || *deployment.Spec.Replicas > 0) {
			zero := int32(0)
			deployment.Spec.Replicas = &zero
			isChanged = true
		}
	}
	if isChanged {
		if err := r.update(csv); err != nil {
			return fmt.Errorf("Failed to update MCG CSV: %v", err)
		}
	}
	return nil
}

func (r *ManagedFusionOfferingReconciler) recocileProviderStorageCluster() error {
	r.Log.Info("Reconciling StorageCluster")

	_, err := ctrl.CreateOrUpdate(r.ctx, r.Client, r.storageCluster, func() error {
		if err := r.own(r.storageCluster); err != nil {
			return err
		}

		sizeAsString := r.managedFusionOffering.Spec.Config.UsableCapacity

		r.Log.Info("Requested offering config", storageSizeKey, sizeAsString)
		desiredSize, err := strconv.Atoi(sizeAsString)
		if err != nil {
			return fmt.Errorf("Invalid storage cluster size value: %v", sizeAsString)
		}

		// Convert the desired size to the device set count based on the underlaying OSD size
		desiredDeviceSetCount := int(math.Ceil(float64(desiredSize) / templates.ProviderOSDSizeInTiB))

		// Get the storage device set count of the current storage cluster
		currDeviceSetCount := 0
		if desiredStorageDeviceSet := findStorageDeviceSet(r.storageCluster.Spec.StorageDeviceSets, deviceSetName); desiredStorageDeviceSet != nil {
			currDeviceSetCount = desiredStorageDeviceSet.Count
		}

		// Get the desired storage device set from storage cluster template
		sc := templates.ProviderStorageClusterTemplate.DeepCopy()
		var ds *ocsv1.StorageDeviceSet = nil
		if desiredStorageDeviceSet := findStorageDeviceSet(sc.Spec.StorageDeviceSets, deviceSetName); desiredStorageDeviceSet != nil {
			ds = desiredStorageDeviceSet
		}

		// Prevent downscaling by comparing count from secret and count from storage cluster
		r.setDeviceSetCount(ds, desiredDeviceSetCount, currDeviceSetCount)

		r.storageCluster.Spec = sc.Spec
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *ManagedFusionOfferingReconciler) reconcileRookCephOperatorConfig() error {
	rookConfigMap := &corev1.ConfigMap{}
	rookConfigMap.Name = rookConfigMapName
	rookConfigMap.Namespace = r.namespace.Name

	if err := r.get(rookConfigMap); err != nil {
		// Because resource limits will not be set, failure to get the Rook ConfigMap results in failure to reconcile.
		return fmt.Errorf("Failed to get Rook ConfigMap: %v", err)
	}

	cloneRookConfigMap := rookConfigMap.DeepCopy()
	if cloneRookConfigMap.Data == nil {
		cloneRookConfigMap.Data = map[string]string{}
	}

	cloneRookConfigMap.Data["ROOK_CSI_ENABLE_CEPHFS"] = "false"
	cloneRookConfigMap.Data["ROOK_CSI_ENABLE_RBD"] = "false"

	if !equality.Semantic.DeepEqual(rookConfigMap, cloneRookConfigMap) {
		if err := r.update(cloneRookConfigMap); err != nil {
			return fmt.Errorf("Failed to update Rook ConfigMap: %v", err)
		}
	}

	return nil
}

func (r *ManagedFusionOfferingReconciler) reconcileOCSInitialization() error {
	r.Log.Info("Reconciling OCSInitialization")

	ocsInitList := ocsv1.OCSInitializationList{}
	if err := r.list(&ocsInitList); err != nil {
		return fmt.Errorf("Could to list OCSInitialization resources: %v", err)
	}
	if len(ocsInitList.Items) == 0 {
		r.Log.V(-1).Info("OCSInitialization resource not found")
	} else {
		obj := &ocsInitList.Items[0]
		if !obj.Spec.EnableCephTools {
			obj.Spec.EnableCephTools = true
			if err := r.update(obj); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ManagedFusionOfferingReconciler) setDeviceSetCount(deviceSet *ocsv1.StorageDeviceSet, desiredDeviceSetCount int, currDeviceSetCount int) {
	r.Log.Info("Setting storage device set count", "Current", currDeviceSetCount, "New", desiredDeviceSetCount)
	if currDeviceSetCount <= desiredDeviceSetCount {
		deviceSet.Count = desiredDeviceSetCount
	} else {
		r.Log.V(-1).Info("Requested storage device set count will result in downscaling, which is not supported. Skipping")
		deviceSet.Count = currDeviceSetCount
	}
}

func findStorageDeviceSet(storageDeviceSets []ocsv1.StorageDeviceSet, deviceSetName string) *ocsv1.StorageDeviceSet {
	for index := range storageDeviceSets {
		item := &storageDeviceSets[index]
		if item.Name == deviceSetName {
			return item
		}
	}
	return nil
}

func (r *ManagedFusionOfferingReconciler) own(resource metav1.Object) error {
	if err := ctrl.SetControllerReference(r.managedFusionOffering, resource, r.Scheme); err != nil {
		return err
	}
	return nil
}

func (r *ManagedFusionOfferingReconciler) get(obj client.Object) error {
	key := client.ObjectKeyFromObject(obj)
	return r.Client.Get(r.ctx, key, obj)
}

func (r *ManagedFusionOfferingReconciler) list(obj client.ObjectList) error {
	listOptions := client.InNamespace(r.namespace.Name)
	return r.Client.List(r.ctx, obj, listOptions)
}

func (r *ManagedFusionOfferingReconciler) update(obj client.Object) error {
	return r.Client.Update(r.ctx, obj)
}
