/*
Copyright (c) 2020 Red Hat, Inc.

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

// IMPORTANT: This file has been generated automatically, refrain from modifying it manually as all
// your changes will be lost when the file is generated again.

package v1 // github.com/openshift-online/ocm-api-model/clientapi/clustersmgmt/v1

import (
	time "time"
)

// Representation of one upgrade scheduled somewhere in the fleet, regardless of
// which kind of upgrade policy scheduled it. Read-only; the authoritative
// resource is linked from 'policy'.
type ScheduledUpgradeBuilder struct {
	fieldSet_    []bool
	id           string
	href         string
	cluster      *ClusterBuilder
	createdAt    time.Time
	nextRun      time.Time
	nodePool     *NodePoolBuilder
	policy       *ScheduledUpgradePolicyLinkBuilder
	scheduleType ScheduleType
	state        *UpgradePolicyStateBuilder
	type_        UpgradeType
	updatedAt    time.Time
	version      string
}

// NewScheduledUpgrade creates a new builder of 'scheduled_upgrade' objects.
func NewScheduledUpgrade() *ScheduledUpgradeBuilder {
	return &ScheduledUpgradeBuilder{
		fieldSet_: make([]bool, 13),
	}
}

// Link sets the flag that indicates if this is a link.
func (b *ScheduledUpgradeBuilder) Link(value bool) *ScheduledUpgradeBuilder {
	if len(b.fieldSet_) == 0 {
		b.fieldSet_ = make([]bool, 13)
	}
	b.fieldSet_[0] = true
	return b
}

// ID sets the identifier of the object.
func (b *ScheduledUpgradeBuilder) ID(value string) *ScheduledUpgradeBuilder {
	if len(b.fieldSet_) == 0 {
		b.fieldSet_ = make([]bool, 13)
	}
	b.id = value
	b.fieldSet_[1] = true
	return b
}

// HREF sets the link to the object.
func (b *ScheduledUpgradeBuilder) HREF(value string) *ScheduledUpgradeBuilder {
	if len(b.fieldSet_) == 0 {
		b.fieldSet_ = make([]bool, 13)
	}
	b.href = value
	b.fieldSet_[2] = true
	return b
}

// Empty returns true if the builder is empty, i.e. no attribute has a value.
func (b *ScheduledUpgradeBuilder) Empty() bool {
	if b == nil || len(b.fieldSet_) == 0 {
		return true
	}
	// Check all fields except the link flag (index 0)
	for i := 1; i < len(b.fieldSet_); i++ {
		if b.fieldSet_[i] {
			return false
		}
	}
	return true
}

// Cluster sets the value of the 'cluster' attribute to the given value.
//
// Definition of an _OpenShift_ cluster.
//
// The `cloud_provider` attribute is a reference to the cloud provider. When a
// cluster is retrieved it will be a link to the cloud provider, containing only
// the kind, id and href attributes:
//
// ```json
//
//	{
//	  "cloud_provider": {
//	    "kind": "CloudProviderLink",
//	    "id": "123",
//	    "href": "/api/clusters_mgmt/v1/cloud_providers/123"
//	  }
//	}
//
// ```
//
// When a cluster is created this is optional, and if used it should contain the
// identifier of the cloud provider to use:
//
// ```json
//
//	{
//	  "cloud_provider": {
//	    "id": "123",
//	  }
//	}
//
// ```
//
// If not included, then the cluster will be created using the default cloud
// provider, which is currently Amazon Web Services.
//
// The region attribute is mandatory when a cluster is created.
//
// The `aws.access_key_id`, `aws.secret_access_key` and `dns.base_domain`
// attributes are mandatory when creation a cluster with your own Amazon Web
// Services account.
func (b *ScheduledUpgradeBuilder) Cluster(value *ClusterBuilder) *ScheduledUpgradeBuilder {
	if len(b.fieldSet_) == 0 {
		b.fieldSet_ = make([]bool, 13)
	}
	b.cluster = value
	if value != nil {
		b.fieldSet_[3] = true
	} else {
		b.fieldSet_[3] = false
	}
	return b
}

// CreatedAt sets the value of the 'created_at' attribute to the given value.
func (b *ScheduledUpgradeBuilder) CreatedAt(value time.Time) *ScheduledUpgradeBuilder {
	if len(b.fieldSet_) == 0 {
		b.fieldSet_ = make([]bool, 13)
	}
	b.createdAt = value
	b.fieldSet_[4] = true
	return b
}

// NextRun sets the value of the 'next_run' attribute to the given value.
func (b *ScheduledUpgradeBuilder) NextRun(value time.Time) *ScheduledUpgradeBuilder {
	if len(b.fieldSet_) == 0 {
		b.fieldSet_ = make([]bool, 13)
	}
	b.nextRun = value
	b.fieldSet_[5] = true
	return b
}

// NodePool sets the value of the 'node_pool' attribute to the given value.
//
// Representation of a node pool in a cluster.
func (b *ScheduledUpgradeBuilder) NodePool(value *NodePoolBuilder) *ScheduledUpgradeBuilder {
	if len(b.fieldSet_) == 0 {
		b.fieldSet_ = make([]bool, 13)
	}
	b.nodePool = value
	if value != nil {
		b.fieldSet_[6] = true
	} else {
		b.fieldSet_[6] = false
	}
	return b
}

// Policy sets the value of the 'policy' attribute to the given value.
//
// Reference to one of the three kinds of upgrade policy resource. Kind
// identifies which policy type it is; id and href come from the standard
// metadata added by the generator.
func (b *ScheduledUpgradeBuilder) Policy(value *ScheduledUpgradePolicyLinkBuilder) *ScheduledUpgradeBuilder {
	if len(b.fieldSet_) == 0 {
		b.fieldSet_ = make([]bool, 13)
	}
	b.policy = value
	if value != nil {
		b.fieldSet_[7] = true
	} else {
		b.fieldSet_[7] = false
	}
	return b
}

// ScheduleType sets the value of the 'schedule_type' attribute to the given value.
//
// ScheduleType defines which type of scheduling should be used for the upgrade policy.
func (b *ScheduledUpgradeBuilder) ScheduleType(value ScheduleType) *ScheduledUpgradeBuilder {
	if len(b.fieldSet_) == 0 {
		b.fieldSet_ = make([]bool, 13)
	}
	b.scheduleType = value
	b.fieldSet_[8] = true
	return b
}

// State sets the value of the 'state' attribute to the given value.
//
// Representation of an upgrade policy state that that is set for a cluster.
func (b *ScheduledUpgradeBuilder) State(value *UpgradePolicyStateBuilder) *ScheduledUpgradeBuilder {
	if len(b.fieldSet_) == 0 {
		b.fieldSet_ = make([]bool, 13)
	}
	b.state = value
	if value != nil {
		b.fieldSet_[9] = true
	} else {
		b.fieldSet_[9] = false
	}
	return b
}

// Type sets the value of the 'type' attribute to the given value.
//
// UpgradeType defines which type of upgrade should be used.
func (b *ScheduledUpgradeBuilder) Type(value UpgradeType) *ScheduledUpgradeBuilder {
	if len(b.fieldSet_) == 0 {
		b.fieldSet_ = make([]bool, 13)
	}
	b.type_ = value
	b.fieldSet_[10] = true
	return b
}

// UpdatedAt sets the value of the 'updated_at' attribute to the given value.
func (b *ScheduledUpgradeBuilder) UpdatedAt(value time.Time) *ScheduledUpgradeBuilder {
	if len(b.fieldSet_) == 0 {
		b.fieldSet_ = make([]bool, 13)
	}
	b.updatedAt = value
	b.fieldSet_[11] = true
	return b
}

// Version sets the value of the 'version' attribute to the given value.
func (b *ScheduledUpgradeBuilder) Version(value string) *ScheduledUpgradeBuilder {
	if len(b.fieldSet_) == 0 {
		b.fieldSet_ = make([]bool, 13)
	}
	b.version = value
	b.fieldSet_[12] = true
	return b
}

// Copy copies the attributes of the given object into this builder, discarding any previous values.
func (b *ScheduledUpgradeBuilder) Copy(object *ScheduledUpgrade) *ScheduledUpgradeBuilder {
	if object == nil {
		return b
	}
	if len(object.fieldSet_) > 0 {
		b.fieldSet_ = make([]bool, len(object.fieldSet_))
		copy(b.fieldSet_, object.fieldSet_)
	}
	b.id = object.id
	b.href = object.href
	if object.cluster != nil {
		b.cluster = NewCluster().Copy(object.cluster)
	} else {
		b.cluster = nil
	}
	b.createdAt = object.createdAt
	b.nextRun = object.nextRun
	if object.nodePool != nil {
		b.nodePool = NewNodePool().Copy(object.nodePool)
	} else {
		b.nodePool = nil
	}
	if object.policy != nil {
		b.policy = NewScheduledUpgradePolicyLink().Copy(object.policy)
	} else {
		b.policy = nil
	}
	b.scheduleType = object.scheduleType
	if object.state != nil {
		b.state = NewUpgradePolicyState().Copy(object.state)
	} else {
		b.state = nil
	}
	b.type_ = object.type_
	b.updatedAt = object.updatedAt
	b.version = object.version
	return b
}

// Build creates a 'scheduled_upgrade' object using the configuration stored in the builder.
func (b *ScheduledUpgradeBuilder) Build() (object *ScheduledUpgrade, err error) {
	object = new(ScheduledUpgrade)
	object.id = b.id
	object.href = b.href
	if len(b.fieldSet_) > 0 {
		object.fieldSet_ = make([]bool, len(b.fieldSet_))
		copy(object.fieldSet_, b.fieldSet_)
	}
	if b.cluster != nil {
		object.cluster, err = b.cluster.Build()
		if err != nil {
			return
		}
	}
	object.createdAt = b.createdAt
	object.nextRun = b.nextRun
	if b.nodePool != nil {
		object.nodePool, err = b.nodePool.Build()
		if err != nil {
			return
		}
	}
	if b.policy != nil {
		object.policy, err = b.policy.Build()
		if err != nil {
			return
		}
	}
	object.scheduleType = b.scheduleType
	if b.state != nil {
		object.state, err = b.state.Build()
		if err != nil {
			return
		}
	}
	object.type_ = b.type_
	object.updatedAt = b.updatedAt
	object.version = b.version
	return
}
