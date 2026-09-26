#! /bin/bash

echo "::group::==== Klusterlet CRs ===="
kubectl get klusterlets -A -o yaml
echo "::endgroup::"

echo "::group::==== Klusterlet CRD ===="
kubectl get crd klusterlets.operator.open-cluster-management.io -o yaml
echo "::endgroup::"

echo "::group::==== ManifestWorks ===="
kubectl get manifestworks -A -o yaml
echo "::endgroup::"

echo "::group::==== AppliedManifestWorks ===="
kubectl get appliedmanifestworks -A -o yaml
echo "::endgroup::"

echo "::group::==== ManagedClusters ===="
kubectl get managedclusters -A -o yaml
echo "::endgroup::"

echo "::group::==== Managed Cluster Leases ===="
for ns in $(kubectl get managedclusters -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
  echo "---- Leases in namespace: $ns ----"
  kubectl get lease -n "$ns" -o yaml
done
echo "::endgroup::"

echo "::group::==== Namespace open-cluster-management-agent ===="
kubectl get ns open-cluster-management-agent -o yaml
echo "::endgroup::"

echo "::group::==== Namespace open-cluster-management-local ===="
kubectl get ns open-cluster-management-local -o yaml
echo "::endgroup::"

echo "::group::==== Pods in open-cluster-management-agent ===="
kubectl get pods -n open-cluster-management-agent -o wide
echo "::endgroup::"

echo "::group::==== Pods in open-cluster-management-local ===="
kubectl get pods -n open-cluster-management-local -o wide
echo "::endgroup::"

echo "::group::==== Klusterlet Operator Pods ===="
kubectl get pods -n open-cluster-management -l app=klusterlet -o wide
echo "::endgroup::"

echo "::group::==== Events in open-cluster-management-agent ===="
kubectl get events -n open-cluster-management-agent --sort-by='.lastTimestamp'
echo "::endgroup::"

echo "::group::==== Events in open-cluster-management ===="
kubectl get events -n open-cluster-management --sort-by='.lastTimestamp' | tail -50
echo "::endgroup::"

echo "::group::==== Logs from pods in open-cluster-management-agent ===="
for pod in $(kubectl get pods -n open-cluster-management-agent -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
  echo "---- Logs from pod: $pod ----"
  kubectl logs -n open-cluster-management-agent "$pod" --all-containers --tail=500
  echo "---- Previous logs from pod: $pod ----"
  kubectl logs -n open-cluster-management-agent "$pod" --all-containers --previous --tail=500
done
echo "::endgroup::"

echo "::group::==== Logs from pods in open-cluster-management-local ===="
for pod in $(kubectl get pods -n open-cluster-management-local -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
  echo "---- Logs from pod: $pod ----"
  kubectl logs -n open-cluster-management-local "$pod" --all-containers --tail=500
  echo "---- Previous logs from pod: $pod ----"
  kubectl logs -n open-cluster-management-local "$pod" --all-containers --previous --tail=500
done
echo "::endgroup::"

echo "::group::==== Klusterlet Operator Logs ===="
kubectl -n open-cluster-management logs -l app=klusterlet --tail=500
echo "::endgroup::"

echo "::group::==== Import Controller Logs ===="
kubectl -n open-cluster-management logs -l name=managedcluster-import-controller --tail=100
echo "::endgroup::"
