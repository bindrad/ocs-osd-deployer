package readiness

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/red-hat-storage/ocs-osd-deployer/api/v1alpha1"
	utils "github.com/red-hat-storage/ocs-osd-deployer/testutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ManagedOCS readiness probe behavior", func() {
	ctx := context.Background()

	managedOCS := &v1.ManagedOCS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ManagedOCSName,
			Namespace: TestNamespace,
		},
	}

	setupReadinessConditions := func(
		prometheusReady bool,
		alertmanagerReady bool,
	) error {

		var prometheusStatus, alertmanagerStatus v1.ComponentState

		if prometheusReady == true {
			prometheusStatus = v1.ComponentReady
		} else {
			prometheusStatus = v1.ComponentPending
		}

		if alertmanagerReady == true {
			alertmanagerStatus = v1.ComponentReady
		} else {
			alertmanagerStatus = v1.ComponentPending
		}

		if err := k8sClient.Get(ctx, utils.GetResourceKey(managedOCS), managedOCS); err != nil {
			return err
		}

		managedOCS.Status = v1.ManagedOCSStatus{
			Components: v1.ComponentStatusMap{
				Prometheus: v1.ComponentStatus{
					State: prometheusStatus,
				},
				Alertmanager: v1.ComponentStatus{
					State: alertmanagerStatus,
				},
			},
		}

		return k8sClient.Status().Update(ctx, managedOCS)
	}

	Context("Readiness Probe", func() {
		When("managedocs reports Prometheus as not \"ready\"", func() {
			It("should cause the readiness probe to return StatusServiceUnavailable", func() {
				Expect(setupReadinessConditions(false, true)).Should(Succeed())

				status, err := utils.ProbeReadiness()
				Expect(err).ToNot(HaveOccurred())
				Expect(status).To(Equal(http.StatusServiceUnavailable))
			})
		})

		When("managedocs reports Alertmanager as not \"ready\"", func() {
			It("should cause the readiness probe to return StatusServiceUnavailable", func() {
				Expect(setupReadinessConditions(true, false)).Should(Succeed())

				status, err := utils.ProbeReadiness()
				Expect(err).ToNot(HaveOccurred())
				Expect(status).To(Equal(http.StatusServiceUnavailable))
			})
		})

		When("managedocs reports all its components as \"ready\"", func() {
			It("should cause the readiness probe to return StatusOK", func() {
				Expect(setupReadinessConditions(true, true)).Should(Succeed())

				status, err := utils.ProbeReadiness()
				Expect(err).ToNot(HaveOccurred())
				Expect(status).To(Equal(http.StatusOK))
			})
		})

	})
})
