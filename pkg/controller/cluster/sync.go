package cluster

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	scyllav1alpha1 "github.com/scylladb/scylla-operator/pkg/apis/scylla/v1alpha1"
	"github.com/scylladb/scylla-operator/pkg/controller/cluster/actions"
	"github.com/scylladb/scylla-operator/pkg/controller/cluster/util"
	"github.com/scylladb/scylla-operator/pkg/naming"
	corev1 "k8s.io/api/core/v1"
)

const (
	// Messages to display when experiencing an error.
	MessageHeadlessServiceSyncFailed = "Failed to sync Headless Service for cluster"
	MessageMemberServicesSyncFailed  = "Failed to sync MemberServices for cluster"
	MessageUpdateStatusFailed        = "Failed to update status for cluster"
	MessageCleanupFailed             = "Failed to clean up cluster resources"
	MessageClusterSyncFailed         = "Failed to sync cluster, got error: %+v"
)

// sync attempts to sync the given Scylla Cluster.
// NOTE: the Cluster Object is a DeepCopy. Modify at will.
func (cc *ClusterController) sync(c *scyllav1alpha1.Cluster) error {

	logger := util.LoggerForCluster(c)
	logger.Info("\nStarting reconciliation...")
	logger.Debug("Cluster Object:")
	logger.Debugf("%+v", spew.Sdump(c))

	// Before syncing, ensure that all StatefulSets are up-to-date
	stale, err := util.AreStatefulSetStatusesStale(c, cc.Client)
	if err != nil {
		return errors.Wrap(err, "failed to check sts staleness")
	}
	if stale {
		return nil
	}
	logger.Info("All StatefulSets are up-to-date!")

	// Cleanup Cluster resources
	if err := cc.cleanup(c); err != nil {
		cc.Recorder.Event(
			c,
			corev1.EventTypeWarning,
			naming.ErrSyncFailed,
			MessageCleanupFailed,
		)
	}

	// Sync Headless Service for Cluster
	if err := cc.syncClusterHeadlessService(c); err != nil {
		cc.Recorder.Event(
			c,
			corev1.EventTypeWarning,
			naming.ErrSyncFailed,
			MessageHeadlessServiceSyncFailed,
		)
		return errors.Wrap(err, "failed to sync headless service")
	}

	// Sync Cluster Member Services
	if err := cc.syncMemberServices(c); err != nil {
		cc.Recorder.Event(
			c,
			corev1.EventTypeWarning,
			naming.ErrSyncFailed,
			MessageMemberServicesSyncFailed,
		)
		return errors.Wrap(err, "failed to sync member service")
	}

	// Update Status
	if err := cc.updateStatus(c); err != nil {
		cc.Recorder.Event(
			c,
			corev1.EventTypeWarning,
			naming.ErrSyncFailed,
			MessageUpdateStatusFailed,
		)
		return errors.Wrap(err, "failed to update status")
	}

	// Calculate and execute next action
	if act := cc.nextAction(c); act != nil {
		s := actions.NewState(cc.Client, cc.KubeClient, cc.Recorder)
		err = act.Execute(s)
	}

	if err != nil {
		cc.Recorder.Event(
			c,
			corev1.EventTypeWarning,
			naming.ErrSyncFailed,
			fmt.Sprintf(MessageClusterSyncFailed, errors.Cause(err)))
	}

	return nil
}

func (cc *ClusterController) nextAction(cluster *scyllav1alpha1.Cluster) actions.Action {

	logger := util.LoggerForCluster(cluster)

	// Check if any rack isn't created
	for _, rack := range cluster.Spec.Datacenter.Racks {
		// For each rack, check if a status entry exists
		if _, ok := cluster.Status.Racks[rack.Name]; !ok {
			logger.Infof("Next Action: Create rack %s", rack.Name)
			return actions.NewRackCreateAction(rack, cluster, cc.OperatorImage)
		}
	}

	// Check if there is a scale-down in progress
	for _, rack := range cluster.Spec.Datacenter.Racks {
		if scyllav1alpha1.IsRackConditionTrue(cluster.Status.Racks[rack.Name], scyllav1alpha1.RackConditionTypeMemberLeaving) {
			// Resume scale down
			logger.Infof("Next Action: Scale-Down rack %s", rack.Name)
			return actions.NewRackScaleDownAction(rack, cluster)
		}
	}

	// Check that all racks are ready before taking any action
	for _, rack := range cluster.Spec.Datacenter.Racks {
		rackStatus := cluster.Status.Racks[rack.Name]
		if rackStatus.Members != rackStatus.ReadyMembers {
			logger.Infof("Rack %s is not ready:\n  Members: %d \n  ReadyMembers: %d", rack.Name, rackStatus.Members, rackStatus.ReadyMembers)
			return nil
		}
	}

	// Check if any rack needs to scale down
	for _, rack := range cluster.Spec.Datacenter.Racks {
		if rack.Members < cluster.Status.Racks[rack.Name].Members {
			logger.Infof("Next Action: Scale-Down rack %s", rack.Name)
			return actions.NewRackScaleDownAction(rack, cluster)
		}
	}

	// Check if any rack needs to scale up
	for _, rack := range cluster.Spec.Datacenter.Racks {

		if rack.Members > cluster.Status.Racks[rack.Name].Members {
			logger.Infof("Next Action: Scale-Up rack %s", rack.Name)
			return actions.NewRackScaleUpAction(rack, cluster)
		}
	}

	// Nothing to do
	return nil
}
