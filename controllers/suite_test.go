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
	"io"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	openshiftv1 "github.com/openshift/api/network/v1"
	opv1a1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	ovnv1 "github.com/ovn-org/ovn-kubernetes/go-controller/pkg/crd/egressfirewall/v1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	promv1a1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	odfv1a1 "github.com/red-hat-data-services/odf-operator/api/v1alpha1"
	ocsv1 "github.com/red-hat-storage/ocs-operator/api/v1"
	ocsv1alpha1 "github.com/red-hat-storage/ocs-operator/api/v1alpha1"
	v1 "github.com/red-hat-storage/ocs-osd-deployer/api/v1alpha1"
	"github.com/red-hat-storage/ocs-osd-deployer/templates"
	"github.com/red-hat-storage/ocs-osd-deployer/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg            *rest.Config
	k8sClient      client.Client
	testEnv        *envtest.Environment
	testReconciler *ManagedFusionDeploymentReconciler
	ctx            context.Context
	cancel         context.CancelFunc
	testDeployment string
)

const (
	testPrimaryNamespace             = "primary"
	testSecondaryNamespace           = "secondary"
	testDeployerCSVName              = "ocs-osd-deployer.x.y.z"
	testCustomerNotificationHTMLPath = "../templates/customernotification.html"
	testDeploymentTypeEnvVarName     = "DEPLOYMENT_TYPE"
	testRHOBSEndpointEnvVarName      = "RHOBS_ENDPOINT"
	testRHssoTokenEndpointEnvVarName = "RH_SSO_TOKEN_ENDPOINT"
	testRHOBSSecretName              = "test-addon-prom-remote-write"
)

func TestAPIs(t *testing.T) {

	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	done := make(chan interface{})

	// write logs from reconciler to a log file
	var logFile io.Writer
	logFile, err := os.Create("/tmp/ocs-osd-deployer.log")
	Expect(err).ToNot(HaveOccurred())

	go func(logFile io.Writer) {

		GinkgoWriter.TeeTo(logFile)
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		ctx, cancel = context.WithCancel(context.Background())

		By("bootstrapping test environment")
		testEnv = &envtest.Environment{
			CRDDirectoryPaths: []string{
				filepath.Join("..", "config", "crd", "bases"),
				filepath.Join("..", "shim", "crds"),
			},
			// when tests are re-run even if CRDs are updated, testEnv will only check
			// for CRD existence and so it's better to delete those from etcd after
			// testEnv cleanup
			CRDInstallOptions: envtest.CRDInstallOptions{
				CleanUpAfterUse: true,
			},
		}

		var err error
		cfg, err = testEnv.Start()
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg).ToNot(BeNil())

		err = ocsv1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		err = promv1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		err = promv1a1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		err = v1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		err = opv1a1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		err = openshiftv1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		err = ovnv1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		err = odfv1a1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		err = ocsv1alpha1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		err = configv1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		// +kubebuilder:scaffold:scheme

		// Client to be use by the test code, using a non cached client
		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())
		Expect(k8sClient).ToNot(BeNil())

		k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:    scheme.Scheme,
			Namespace: testPrimaryNamespace,
		})
		Expect(err).ToNot(HaveOccurred())

		testReconciler = &ManagedFusionDeploymentReconciler{
			Client:                       k8sManager.GetClient(),
			UnrestrictedClient:           k8sClient,
			Log:                          ctrl.Log.WithName("controllers").WithName("ManagedOCS"),
			Scheme:                       k8sManager.GetScheme(),
			CustomerNotificationHTMLPath: testCustomerNotificationHTMLPath,
		}

		ctrlOptions := &controller.Options{
			MaxConcurrentReconciles: 1,
			// RateLimiter:             workqueue.NewItemFastSlowRateLimiter(0, 500*time.Millisecond, 0),
		}

		err = (testReconciler).SetupWithManager(k8sManager, ctrlOptions)
		Expect(err).ToNot(HaveOccurred())

		go func() {
			defer GinkgoRecover()
			err = k8sManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred(), "failed to run manager")
		}()

		// Create the primary namespace to be used by some of the tests
		primaryNS := &corev1.Namespace{}
		primaryNS.Name = testPrimaryNamespace
		Expect(k8sClient.Create(ctx, primaryNS)).Should(Succeed())

		// Create a secondary namespace to be used by some of the tests
		secondaryNS := &corev1.Namespace{}
		secondaryNS.Name = testSecondaryNamespace
		Expect(k8sClient.Create(ctx, secondaryNS)).Should(Succeed())

		// Create the aws data config map
		awsConfigMap := &corev1.ConfigMap{}
		awsConfigMap.Name = utils.IMDSConfigMapName
		awsConfigMap.Namespace = testPrimaryNamespace
		awsConfigMap.Data = map[string]string{
			utils.CIDRKey: "10.0.0.0/16",
		}
		Expect(k8sClient.Create(ctx, awsConfigMap)).ShouldNot(HaveOccurred())

		// create a mock deplyer CSV
		deployerCSV := &opv1a1.ClusterServiceVersion{}
		deployerCSV.Name = testDeployerCSVName
		deployerCSV.Namespace = testPrimaryNamespace
		deployerCSV.Spec.InstallStrategy.StrategyName = "test-strategy"
		deployerCSV.Spec.InstallStrategy.StrategySpec.DeploymentSpecs = []opv1a1.StrategyDeploymentSpec{
			{
				Name: "ocs-osd-controller-manager",
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "managedocs"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "managedocs"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "kube-rbac-proxy",
									Image: "test",
								},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deployerCSV)).ShouldNot(HaveOccurred())

		// Create a mock install plan
		prometheusInstallPlan := &opv1a1.InstallPlan{}
		prometheusInstallPlan.Name = "test-install-plan"
		prometheusInstallPlan.Namespace = testPrimaryNamespace
		prometheusInstallPlan.Spec = getMockPrometheusInstallPlanSpec()
		Expect(k8sClient.Create(ctx, prometheusInstallPlan)).ShouldNot(HaveOccurred())

		prometheusCSV := &opv1a1.ClusterServiceVersion{}
		prometheusCSV.Name = templates.PrometheusCSVName
		prometheusCSV.Namespace = testPrimaryNamespace
		prometheusCSV.Spec.InstallStrategy.StrategyName = "test-strategy"
		prometheusCSV.Spec.InstallStrategy.StrategySpec.DeploymentSpecs = getMockPrometheusCSVDeploymentSpec()
		Expect(k8sClient.Create(ctx, prometheusCSV)).ShouldNot(HaveOccurred())

		// Create the ManagedOCS resource
		// managedFusionDeployment := &v1.ManagedFusionDeployment{}
		// managedFusionDeployment.Name = "test-deployment"
		// managedFusionDeployment.Namespace = testPrimaryNamespace
		// Expect(k8sClient.Create(ctx, managedFusionDeployment)).ShouldNot(HaveOccurred())

		close(done)
	}(logFile)
	Eventually(done, 60).Should(BeClosed())
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
	GinkgoWriter.ClearTeeWriters()
})

func getMockPrometheusInstallPlanSpec() opv1a1.InstallPlanSpec {
	return opv1a1.InstallPlanSpec{
		CatalogSource:          prometheusCatalogSourceName,
		CatalogSourceNamespace: testPrimaryNamespace,
		ClusterServiceVersionNames: []string{
			templates.PrometheusCSVName,
		},
		Approval: opv1a1.ApprovalManual,
		Approved: false,
	}
}

func getMockPrometheusCSVDeploymentSpec() []opv1a1.StrategyDeploymentSpec {
	deploymentSpec := []opv1a1.StrategyDeploymentSpec{
		{
			Name: "prometheus-operator",
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"prometheus-operator": "deployment"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"prometheus-operator": "deployment"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "prometheus-operator",
							},
						},
					},
				},
			},
		},
	}
	return deploymentSpec
}

func getMockOCSCSVDeploymentSpec() []opv1a1.StrategyDeploymentSpec {
	deploymentSpec := []opv1a1.StrategyDeploymentSpec{
		{
			Name: "deployment1",
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "ocs-operator"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "ocs-operator"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "ocs-operator",
							},
						},
					},
				},
			},
		},
		{
			Name: "deployment2",
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "rook-ceph-operator"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "rook-ceph-operator"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "rook-ceph-operator",
							},
						},
					},
				},
			},
		},
		{
			Name: "deployment3",
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "ocs-metrics-exporter"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "ocs-metrics-exporter"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "ocs-metrics-exporter",
							},
						},
					},
				},
			},
		},
	}
	return deploymentSpec
}

func getMockMCGCSVDeploymentSpec() []opv1a1.StrategyDeploymentSpec {
	deploymentSpec := []opv1a1.StrategyDeploymentSpec{
		{
			Name: "noobaa-operator",
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"noobaa-operator": "deployment"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"noobaa-operator": "deployment"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "noobaa-operator",
							},
						},
					},
				},
			},
		},
	}
	return deploymentSpec
}
