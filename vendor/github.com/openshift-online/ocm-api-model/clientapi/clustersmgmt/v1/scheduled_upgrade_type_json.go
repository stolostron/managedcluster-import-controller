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
	"io"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/openshift-online/ocm-api-model/clientapi/helpers"
)

// MarshalScheduledUpgrade writes a value of the 'scheduled_upgrade' type to the given writer.
func MarshalScheduledUpgrade(object *ScheduledUpgrade, writer io.Writer) error {
	stream := helpers.NewStream(writer)
	WriteScheduledUpgrade(object, stream)
	err := stream.Flush()
	if err != nil {
		return err
	}
	return stream.Error
}

// WriteScheduledUpgrade writes a value of the 'scheduled_upgrade' type to the given stream.
func WriteScheduledUpgrade(object *ScheduledUpgrade, stream *jsoniter.Stream) {
	count := 0
	stream.WriteObjectStart()
	stream.WriteObjectField("kind")
	if len(object.fieldSet_) > 0 && object.fieldSet_[0] {
		stream.WriteString(ScheduledUpgradeLinkKind)
	} else {
		stream.WriteString(ScheduledUpgradeKind)
	}
	count++
	if len(object.fieldSet_) > 1 && object.fieldSet_[1] {
		if count > 0 {
			stream.WriteMore()
		}
		stream.WriteObjectField("id")
		stream.WriteString(object.id)
		count++
	}
	if len(object.fieldSet_) > 2 && object.fieldSet_[2] {
		if count > 0 {
			stream.WriteMore()
		}
		stream.WriteObjectField("href")
		stream.WriteString(object.href)
		count++
	}
	var present_ bool
	present_ = len(object.fieldSet_) > 3 && object.fieldSet_[3] && object.cluster != nil
	if present_ {
		if count > 0 {
			stream.WriteMore()
		}
		stream.WriteObjectField("cluster")
		WriteCluster(object.cluster, stream)
		count++
	}
	present_ = len(object.fieldSet_) > 4 && object.fieldSet_[4]
	if present_ {
		if count > 0 {
			stream.WriteMore()
		}
		stream.WriteObjectField("created_at")
		stream.WriteString((object.createdAt).Format(time.RFC3339))
		count++
	}
	present_ = len(object.fieldSet_) > 5 && object.fieldSet_[5]
	if present_ {
		if count > 0 {
			stream.WriteMore()
		}
		stream.WriteObjectField("next_run")
		stream.WriteString((object.nextRun).Format(time.RFC3339))
		count++
	}
	present_ = len(object.fieldSet_) > 6 && object.fieldSet_[6] && object.nodePool != nil
	if present_ {
		if count > 0 {
			stream.WriteMore()
		}
		stream.WriteObjectField("node_pool")
		WriteNodePool(object.nodePool, stream)
		count++
	}
	present_ = len(object.fieldSet_) > 7 && object.fieldSet_[7] && object.policy != nil
	if present_ {
		if count > 0 {
			stream.WriteMore()
		}
		stream.WriteObjectField("policy")
		WriteScheduledUpgradePolicyLink(object.policy, stream)
		count++
	}
	present_ = len(object.fieldSet_) > 8 && object.fieldSet_[8]
	if present_ {
		if count > 0 {
			stream.WriteMore()
		}
		stream.WriteObjectField("schedule_type")
		stream.WriteString(string(object.scheduleType))
		count++
	}
	present_ = len(object.fieldSet_) > 9 && object.fieldSet_[9] && object.state != nil
	if present_ {
		if count > 0 {
			stream.WriteMore()
		}
		stream.WriteObjectField("state")
		WriteUpgradePolicyState(object.state, stream)
		count++
	}
	present_ = len(object.fieldSet_) > 10 && object.fieldSet_[10]
	if present_ {
		if count > 0 {
			stream.WriteMore()
		}
		stream.WriteObjectField("type")
		stream.WriteString(string(object.type_))
		count++
	}
	present_ = len(object.fieldSet_) > 11 && object.fieldSet_[11]
	if present_ {
		if count > 0 {
			stream.WriteMore()
		}
		stream.WriteObjectField("updated_at")
		stream.WriteString((object.updatedAt).Format(time.RFC3339))
		count++
	}
	present_ = len(object.fieldSet_) > 12 && object.fieldSet_[12]
	if present_ {
		if count > 0 {
			stream.WriteMore()
		}
		stream.WriteObjectField("version")
		stream.WriteString(object.version)
	}
	stream.WriteObjectEnd()
}

// UnmarshalScheduledUpgrade reads a value of the 'scheduled_upgrade' type from the given
// source, which can be an slice of bytes, a string or a reader.
func UnmarshalScheduledUpgrade(source interface{}) (object *ScheduledUpgrade, err error) {
	iterator, err := helpers.NewIterator(source)
	if err != nil {
		return
	}
	object = ReadScheduledUpgrade(iterator)
	err = iterator.Error
	return
}

// ReadScheduledUpgrade reads a value of the 'scheduled_upgrade' type from the given iterator.
func ReadScheduledUpgrade(iterator *jsoniter.Iterator) *ScheduledUpgrade {
	object := &ScheduledUpgrade{
		fieldSet_: make([]bool, 13),
	}
	for {
		field := iterator.ReadObject()
		if field == "" {
			break
		}
		switch field {
		case "kind":
			value := iterator.ReadString()
			if value == ScheduledUpgradeLinkKind {
				object.fieldSet_[0] = true
			}
		case "id":
			object.id = iterator.ReadString()
			object.fieldSet_[1] = true
		case "href":
			object.href = iterator.ReadString()
			object.fieldSet_[2] = true
		case "cluster":
			value := ReadCluster(iterator)
			object.cluster = value
			object.fieldSet_[3] = true
		case "created_at":
			text := iterator.ReadString()
			value, err := time.Parse(time.RFC3339, text)
			if err != nil {
				iterator.ReportError("", err.Error())
			}
			object.createdAt = value
			object.fieldSet_[4] = true
		case "next_run":
			text := iterator.ReadString()
			value, err := time.Parse(time.RFC3339, text)
			if err != nil {
				iterator.ReportError("", err.Error())
			}
			object.nextRun = value
			object.fieldSet_[5] = true
		case "node_pool":
			value := ReadNodePool(iterator)
			object.nodePool = value
			object.fieldSet_[6] = true
		case "policy":
			value := ReadScheduledUpgradePolicyLink(iterator)
			object.policy = value
			object.fieldSet_[7] = true
		case "schedule_type":
			text := iterator.ReadString()
			value := ScheduleType(text)
			object.scheduleType = value
			object.fieldSet_[8] = true
		case "state":
			value := ReadUpgradePolicyState(iterator)
			object.state = value
			object.fieldSet_[9] = true
		case "type":
			text := iterator.ReadString()
			value := UpgradeType(text)
			object.type_ = value
			object.fieldSet_[10] = true
		case "updated_at":
			text := iterator.ReadString()
			value, err := time.Parse(time.RFC3339, text)
			if err != nil {
				iterator.ReportError("", err.Error())
			}
			object.updatedAt = value
			object.fieldSet_[11] = true
		case "version":
			value := iterator.ReadString()
			object.version = value
			object.fieldSet_[12] = true
		default:
			iterator.ReadAny()
		}
	}
	return object
}
