Conditions
==========

Provides:

* `Condition` type as specified in the [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties)
* `ConditionType` and generally useful constants for this type (ie. "Available",
    "Progressing", "Degraded", and "Upgradeable")
* Functions for setting, removing, finding, and evaluating conditions.

To use, simply add `Conditions` to your Custom Resource Status struct like:

```golang
// ExampleAppStatus defines the observed state of ExampleApp
type ExampleAppStatus struct {
  ...
  // conditions describes the state of the operator's reconciliation functionality.
  // +patchMergeKey=type
  // +patchStrategy=merge
  // +optional
  // Conditions is a list of conditions related to operator reconciliation
  Conditions []conditions.Condition `json:"conditions,omitempty"  patchStrategy:"merge" patchMergeKey:"type"`
}
```

Then, as appropriate in your Reconcile function, use
`conditions.SetStatusCondition` like:

```golang
instance := &examplev1alpha1.ExampleApp{}
err := r.client.Get(context.TODO(), request.NamespacedName, instance)
...handle err

changed := conditions.SetStatusCondition(&instance.Status.Conditions, conditions.Condition{
  Type:   conditions.ConditionAvailable,
  Status: corev1.ConditionFalse,
  Reason: "ReconcileStarted",
  Message: "Reconciling resource"
})

// To avoid thrashing, only update the status on the server if SetStatusCondition changed
// something (other than the heartbeat).
if changed {
  err = r.client.Status().Update(context.TODO(), instance)
  ...handle err
}
```
