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

// ScheduledUpgradePolicyLinkKind is the name of the type used to represent objects
// of type 'scheduled_upgrade_policy_link'.
const ScheduledUpgradePolicyLinkKind = "ScheduledUpgradePolicyLink"

// ScheduledUpgradePolicyLinkLinkKind is the name of the type used to represent links
// to objects of type 'scheduled_upgrade_policy_link'.
const ScheduledUpgradePolicyLinkLinkKind = "ScheduledUpgradePolicyLinkLink"

// ScheduledUpgradePolicyLinkNilKind is the name of the type used to nil references
// to objects of type 'scheduled_upgrade_policy_link'.
const ScheduledUpgradePolicyLinkNilKind = "ScheduledUpgradePolicyLinkNil"

// ScheduledUpgradePolicyLink represents the values of the 'scheduled_upgrade_policy_link' type.
//
// Reference to one of the three kinds of upgrade policy resource. Kind
// identifies which policy type it is; id and href come from the standard
// metadata added by the generator.
type ScheduledUpgradePolicyLink struct {
	fieldSet_ []bool
	id        string
	href      string
}

// Kind returns the name of the type of the object.
func (o *ScheduledUpgradePolicyLink) Kind() string {
	if o == nil {
		return ScheduledUpgradePolicyLinkNilKind
	}
	if len(o.fieldSet_) > 0 && o.fieldSet_[0] {
		return ScheduledUpgradePolicyLinkLinkKind
	}
	return ScheduledUpgradePolicyLinkKind
}

// Link returns true if this is a link.
func (o *ScheduledUpgradePolicyLink) Link() bool {
	return o != nil && len(o.fieldSet_) > 0 && o.fieldSet_[0]
}

// ID returns the identifier of the object.
func (o *ScheduledUpgradePolicyLink) ID() string {
	if o != nil && len(o.fieldSet_) > 1 && o.fieldSet_[1] {
		return o.id
	}
	return ""
}

// GetID returns the identifier of the object and a flag indicating if the
// identifier has a value.
func (o *ScheduledUpgradePolicyLink) GetID() (value string, ok bool) {
	ok = o != nil && len(o.fieldSet_) > 1 && o.fieldSet_[1]
	if ok {
		value = o.id
	}
	return
}

// HREF returns the link to the object.
func (o *ScheduledUpgradePolicyLink) HREF() string {
	if o != nil && len(o.fieldSet_) > 2 && o.fieldSet_[2] {
		return o.href
	}
	return ""
}

// GetHREF returns the link of the object and a flag indicating if the
// link has a value.
func (o *ScheduledUpgradePolicyLink) GetHREF() (value string, ok bool) {
	ok = o != nil && len(o.fieldSet_) > 2 && o.fieldSet_[2]
	if ok {
		value = o.href
	}
	return
}

// Empty returns true if the object is empty, i.e. no attribute has a value.
func (o *ScheduledUpgradePolicyLink) Empty() bool {
	if o == nil || len(o.fieldSet_) == 0 {
		return true
	}

	// Check all fields except the link flag (index 0)
	for i := 1; i < len(o.fieldSet_); i++ {
		if o.fieldSet_[i] {
			return false
		}
	}
	return true
}

// ScheduledUpgradePolicyLinkListKind is the name of the type used to represent list of objects of
// type 'scheduled_upgrade_policy_link'.
const ScheduledUpgradePolicyLinkListKind = "ScheduledUpgradePolicyLinkList"

// ScheduledUpgradePolicyLinkListLinkKind is the name of the type used to represent links to list
// of objects of type 'scheduled_upgrade_policy_link'.
const ScheduledUpgradePolicyLinkListLinkKind = "ScheduledUpgradePolicyLinkListLink"

// ScheduledUpgradePolicyLinkNilKind is the name of the type used to nil lists of objects of
// type 'scheduled_upgrade_policy_link'.
const ScheduledUpgradePolicyLinkListNilKind = "ScheduledUpgradePolicyLinkListNil"

// ScheduledUpgradePolicyLinkList is a list of values of the 'scheduled_upgrade_policy_link' type.
type ScheduledUpgradePolicyLinkList struct {
	href  string
	link  bool
	items []*ScheduledUpgradePolicyLink
}

// Kind returns the name of the type of the object.
func (l *ScheduledUpgradePolicyLinkList) Kind() string {
	if l == nil {
		return ScheduledUpgradePolicyLinkListNilKind
	}
	if l.link {
		return ScheduledUpgradePolicyLinkListLinkKind
	}
	return ScheduledUpgradePolicyLinkListKind
}

// Link returns true iif this is a link.
func (l *ScheduledUpgradePolicyLinkList) Link() bool {
	return l != nil && l.link
}

// HREF returns the link to the list.
func (l *ScheduledUpgradePolicyLinkList) HREF() string {
	if l != nil {
		return l.href
	}
	return ""
}

// GetHREF returns the link of the list and a flag indicating if the
// link has a value.
func (l *ScheduledUpgradePolicyLinkList) GetHREF() (value string, ok bool) {
	ok = l != nil && l.href != ""
	if ok {
		value = l.href
	}
	return
}

// Len returns the length of the list.
func (l *ScheduledUpgradePolicyLinkList) Len() int {
	if l == nil {
		return 0
	}
	return len(l.items)
}

// Items sets the items of the list.
func (l *ScheduledUpgradePolicyLinkList) SetLink(link bool) {
	l.link = link
}

// Items sets the items of the list.
func (l *ScheduledUpgradePolicyLinkList) SetHREF(href string) {
	l.href = href
}

// Items sets the items of the list.
func (l *ScheduledUpgradePolicyLinkList) SetItems(items []*ScheduledUpgradePolicyLink) {
	l.items = items
}

// Items returns the items of the list.
func (l *ScheduledUpgradePolicyLinkList) Items() []*ScheduledUpgradePolicyLink {
	if l == nil {
		return nil
	}
	return l.items
}

// Empty returns true if the list is empty.
func (l *ScheduledUpgradePolicyLinkList) Empty() bool {
	return l == nil || len(l.items) == 0
}

// Get returns the item of the list with the given index. If there is no item with
// that index it returns nil.
func (l *ScheduledUpgradePolicyLinkList) Get(i int) *ScheduledUpgradePolicyLink {
	if l == nil || i < 0 || i >= len(l.items) {
		return nil
	}
	return l.items[i]
}

// Slice returns an slice containing the items of the list. The returned slice is a
// copy of the one used internally, so it can be modified without affecting the
// internal representation.
//
// If you don't need to modify the returned slice consider using the Each or Range
// functions, as they don't need to allocate a new slice.
func (l *ScheduledUpgradePolicyLinkList) Slice() []*ScheduledUpgradePolicyLink {
	var slice []*ScheduledUpgradePolicyLink
	if l == nil {
		slice = make([]*ScheduledUpgradePolicyLink, 0)
	} else {
		slice = make([]*ScheduledUpgradePolicyLink, len(l.items))
		copy(slice, l.items)
	}
	return slice
}

// Each runs the given function for each item of the list, in order. If the function
// returns false the iteration stops, otherwise it continues till all the elements
// of the list have been processed.
func (l *ScheduledUpgradePolicyLinkList) Each(f func(item *ScheduledUpgradePolicyLink) bool) {
	if l == nil {
		return
	}
	for _, item := range l.items {
		if !f(item) {
			break
		}
	}
}

// Range runs the given function for each index and item of the list, in order. If
// the function returns false the iteration stops, otherwise it continues till all
// the elements of the list have been processed.
func (l *ScheduledUpgradePolicyLinkList) Range(f func(index int, item *ScheduledUpgradePolicyLink) bool) {
	if l == nil {
		return
	}
	for index, item := range l.items {
		if !f(index, item) {
			break
		}
	}
}
