// +build integration
// Copyright (C) 2017 ScyllaDB

package actions_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/scylladb/go-log"
	"github.com/scylladb/scylla-operator/pkg/controllers/cluster/actions"
	"github.com/scylladb/scylla-operator/pkg/naming"
	"github.com/scylladb/scylla-operator/pkg/scyllaclient"
	testutils "github.com/scylladb/scylla-operator/pkg/test/utils"
	"github.com/scylladb/scylla-operator/pkg/util/httpx"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	scyllav1alpha1 "github.com/scylladb/scylla-operator/pkg/api/v1alpha1"
	"github.com/scylladb/scylla-operator/pkg/test/integration"
)

var _ = Describe("Cluster controller", func() {
	var (
		ns *corev1.Namespace
	)

	BeforeEach(func() {
		var err error
		ns, err = testEnv.CreateNamespace(ctx, "ns")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		Expect(testEnv.Delete(ctx, ns)).To(Succeed())
	})

	Context("Cluster upgrade", func() {
		var (
			scylla  *scyllav1alpha1.ScyllaCluster
			sstStub *integration.StatefulSetOperatorStub

			originalActionsNewSessionFunc             func(hosts []string) (actions.CQLSession, error)
			originalActionsScyllaClientForClusterFunc func(ctx context.Context, cc client.Client, hosts []string, logger log.Logger) (*scyllaclient.Client, error)
		)

		BeforeEach(func() {
			scylla = testEnv.SingleRackCluster(ns)
			scylla.Spec.GenericUpgrade = &scyllav1alpha1.GenericUpgradeSpec{
				PollInterval:      &metav1.Duration{Duration: 200 * time.Millisecond},
				ValidationTimeout: &metav1.Duration{Duration: 5 * time.Second},
			}

			Expect(testEnv.Create(ctx, scylla)).To(Succeed())
			Expect(testEnv.WaitForCluster(ctx, scylla)).To(Succeed())
			Expect(testEnv.Refresh(ctx, scylla)).To(Succeed())

			sstStub = integration.NewStatefulSetOperatorStub(testEnv)

			// Cluster should be scaled sequentially up to member count
			rack := scylla.Spec.Datacenter.Racks[0]
			for _, replicas := range testEnv.ClusterScaleSteps(rack.Members) {
				Expect(testEnv.AssertRackScaled(ctx, rack, scylla, replicas)).To(Succeed())
				Expect(sstStub.CreatePods(ctx, scylla)).To(Succeed())
			}

			originalActionsNewSessionFunc = actions.NewSessionFunc
			originalActionsScyllaClientForClusterFunc = actions.ScyllaClientForClusterFunc
		})

		AfterEach(func() {
			actions.NewSessionFunc = originalActionsNewSessionFunc
			actions.ScyllaClientForClusterFunc = originalActionsScyllaClientForClusterFunc
			Expect(testEnv.Delete(ctx, scylla)).To(Succeed())
		})

		It("Patch version upgrade", func() {
			Expect(testEnv.Refresh(ctx, scylla)).To(Succeed())
			scylla.Spec.Version = "4.2.1"
			Expect(testEnv.Update(ctx, scylla)).To(Succeed())

			Eventually(func() string {
				sts, err := testEnv.StatefulSetOfRack(ctx, scylla.Spec.Datacenter.Racks[0], scylla)
				Expect(err).ToNot(HaveOccurred())

				idx, err := naming.FindScyllaContainer(sts.Spec.Template.Spec.Containers)
				Expect(err).ToNot(HaveOccurred())

				ver, err := naming.ImageToVersion(sts.Spec.Template.Spec.Containers[idx].Image)
				Expect(err).ToNot(HaveOccurred())

				return ver
			}).Should(Equal("4.2.1"))
		})

		It("Major upgrade", func() {
			shortWait := 2 * time.Second

			systemKeyspaces := []string{"system_schema", "system"}
			allKeyspaces := []string{"system_schema", "system", "data_0", "data_1"}

			scyllaFake := integration.NewScyllaFake(scyllaclient.OperationalModeNormal, allKeyspaces)
			scyllaAddr := scyllaFake.Start()

			defer scyllaFake.Close()

			hrt := testutils.NewHackableRoundTripper(scyllaclient.DefaultTransport())
			hrt.SetInterceptor(httpx.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				req.Host = scyllaAddr
				req.URL = &url.URL{
					Scheme:   "http",
					Path:     req.URL.Path,
					Host:     scyllaAddr,
					RawQuery: req.URL.Query().Encode(),
				}
				return http.DefaultClient.Do(req)
			}))

			actions.NewSessionFunc = func(hosts []string) (actions.CQLSession, error) {
				return cqlSessionStub{}, nil
			}

			actions.ScyllaClientForClusterFunc = func(ctx context.Context, cc client.Client, hosts []string, logger log.Logger) (*scyllaclient.Client, error) {
				cfg := scyllaclient.DefaultConfig(scyllaAddr)
				cfg.Transport = hrt
				return scyllaclient.NewClient(cfg, logger)
			}

			By("When: Scylla Cluster major version is upgraded")
			Expect(testEnv.Refresh(ctx, scylla)).To(Succeed())
			scylla.Spec.Version = "5.2.0"
			Expect(testEnv.Update(ctx, scylla)).To(Succeed())

			By("Then: Cluster status should contain upgrade status")
			Eventually(func() *scyllav1alpha1.UpgradeStatus {
				Expect(testEnv.Refresh(ctx, scylla)).To(Succeed())
				return scylla.Status.Upgrade
			}, shortWait).ShouldNot(BeNil())

			By("Then: system keyspaces snapshot is taken")
			Eventually(scyllaFake.KeyspaceSnapshots, shortWait).Should(ConsistOf(systemKeyspaces))

			By("Then: Scylla image is upgraded")
			rack := scylla.Spec.Datacenter.Racks[0]
			Eventually(func() string {
				sts, err := testEnv.StatefulSetOfRack(ctx, rack, scylla)
				Expect(err).ToNot(HaveOccurred())

				idx, err := naming.FindScyllaContainer(sts.Spec.Template.Spec.Containers)
				Expect(err).ToNot(HaveOccurred())

				ver, err := naming.ImageToVersion(sts.Spec.Template.Spec.Containers[idx].Image)
				Expect(err).ToNot(HaveOccurred())

				return ver
			}, shortWait).Should(Equal("5.2.0"))

			for nodeUnderUpgradeIdx := int(rack.Members - 1); nodeUnderUpgradeIdx >= 0; nodeUnderUpgradeIdx-- {
				By(fmt.Sprintf("Then: Pod %d is being upgraded", nodeUnderUpgradeIdx))

				By("Then: maintenance mode is enabled")
				Eventually(func() map[string]string {
					services, err := testEnv.RackMemberServices(ctx, ns.Namespace, rack, scylla)
					Expect(err).ToNot(HaveOccurred())

					for _, s := range services {
						if strings.HasSuffix(s.Name, fmt.Sprintf("%d", nodeUnderUpgradeIdx)) {
							return s.Labels
						}
					}

					return map[string]string{}
				}, shortWait).Should(HaveKeyWithValue(naming.NodeMaintenanceLabel, ""))

				By("Then: node is being drained")
				Eventually(scyllaFake.DrainRequests, shortWait).Should(Equal(int(rack.Members) - nodeUnderUpgradeIdx))

				By("When: node enters UN state")
				scyllaFake.SetOperationalMode(scyllaclient.OperationalModeDrained)

				By("Then: data snapshot is taken")
				Eventually(scyllaFake.KeyspaceSnapshots, shortWait).Should(ConsistOf(allKeyspaces))

				By("Then: maintenance mode is disabled")
				Eventually(func() map[string]string {
					services, err := testEnv.RackMemberServices(ctx, ns.Namespace, rack, scylla)
					Expect(err).ToNot(HaveOccurred())

					for _, s := range services {
						if strings.HasSuffix(s.Name, fmt.Sprintf("%d", nodeUnderUpgradeIdx)) {
							return s.Labels
						}
					}

					return map[string]string{}
				}, shortWait).ShouldNot(HaveKey(naming.NodeMaintenanceLabel))

				By("Then: node pod is deleted")
				Eventually(func() int {
					podList := &corev1.PodList{}
					Expect(testEnv.List(ctx, podList, &client.ListOptions{LabelSelector: naming.RackSelector(rack, scylla)})).To(Succeed())

					return len(podList.Items)
				}, shortWait).Should(Equal(int(rack.Members - 1)))

				By("When: node pod comes up")
				Expect(sstStub.CreatePodsPartition(ctx, scylla, nodeUnderUpgradeIdx)).To(Succeed())

				podList := &corev1.PodList{}
				Expect(testEnv.List(ctx, podList, &client.ListOptions{LabelSelector: naming.RackSelector(rack, scylla)})).To(Succeed())

				By("When: node enters UN state")
				scyllaFake.SetOperationalMode(scyllaclient.OperationalModeNormal)

				By("When: node pod is ready")
				for _, p := range podList.Items {
					if strings.HasSuffix(p.Name, fmt.Sprintf("%d", nodeUnderUpgradeIdx)) {
						found := false
						for i, c := range p.Status.Conditions {
							if c.Type == corev1.PodReady {
								p.Status.Conditions[i].Status = corev1.ConditionTrue
								found = true
							}
						}
						if !found {
							p.Status.Conditions = append(p.Status.Conditions, corev1.PodCondition{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							})
						}

						Expect(testEnv.Status().Update(ctx, &p)).To(Succeed())
					}
				}

				By("Then: data snapshot is removed")
				Eventually(scyllaFake.KeyspaceSnapshots, shortWait).Should(ConsistOf(systemKeyspaces))
			}

			By("Then: system snapshot is removed")
			Eventually(scyllaFake.KeyspaceSnapshots, shortWait).Should(BeEmpty())

			By("Then: upgrade status is cleared out")
			Eventually(func() *scyllav1alpha1.UpgradeStatus {
				Expect(testEnv.Refresh(ctx, scylla)).To(Succeed())
				return scylla.Status.Upgrade
			}, shortWait).Should(BeNil())
		})
	})

})

type cqlSessionStub struct {
}

func (c cqlSessionStub) AwaitSchemaAgreement(ctx context.Context) error {
	// Always succeed
	return nil
}
