# managedcluster-import-controller — Agent Instructions

`managedcluster-import-controller` is a Go controller-runtime service in the Open Cluster Management ecosystem. It imports and detaches `ManagedCluster` resources, installs and updates Klusterlet-related resources, processes import and bootstrap secrets, and supports hosted-cluster and FlightCtl flows.

## Repository Layout

- `cmd/manager/`: main controller process.
- `cmd/tls-profile-sync/`: TLS profile synchronization utility.
- `pkg/controller/`: reconcilers and controller registration.
- `pkg/helpers/`: client, TLS, import configuration, bootstrap, and resource helpers.
- `pkg/bootstrap/`: bootstrap kubeconfig and related manifest rendering.
- `pkg/source/`: informer-backed event sources and mappings.
- `pkg/tlsprofilesync/`: TLS profile synchronization logic.
- `deploy/`: Kustomize manifests and CRDs used for deployment.
- `test/e2e/`: multi-cluster end-to-end tests and test resources.
- `docs/`: import-flow and operational documentation.
- `vendor/`: vendored Go dependencies; do not modify for normal feature work.

## Development Commands

Run commands from the repository root.

```bash
make build                 # builds build/_output/manager and build/_output/tls-profile-sync
make test                  # configures envtest, then runs unit tests with coverage
make lint                  # runs the repository's upstream SDK lint script
make check                 # copyright check and lint
make e2e-test              # provisions kind/OCM and runs the default e2e suite
make e2e-test-core         # runs core e2e tests on a single cluster
make e2e-test-misc         # runs miscellaneous e2e tests on a single cluster
make e2e-test-hosted       # runs hosted-cluster e2e tests
make e2e-test-prow         # runs agent-registration tests against Prow
make clean                 # removes generated output and kind e2e resources
```

`make test` downloads or locates envtest assets through the SDK script and clears the Go build cache before running. `make lint` and the e2e targets download or invoke external tooling, so use them only when network access and the required cluster/container tooling are available. `make build-image` and the e2e targets require a Docker-compatible builder.

For focused Go tests, use the standard package command, for example:

```bash
go test ./pkg/controller/importconfig
go test ./pkg/helpers/...
```

Use `gofmt` on changed Go files. Keep copyright headers consistent with neighboring files and update tests with controller behavior changes.

## Architecture

For system boundaries, data flows, and module layout, see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Configuration and Runtime Notes

- The manager uses Kubernetes configuration, leader election, controller-runtime metrics on port `8383`, and shared informers for secrets, ManifestWorks, `ManagedCluster`, and `KlusterletConfig` resources.
- Controllers are registered centrally in `pkg/controller/controller.go`; feature gates determine whether hosted-mode and related optional behavior is active.
- `ENABLE_KLUSTERLET_NETWORK_POLICIES` can provide the network-policy feature setting when the corresponding command-line flag was not explicitly set.
- `ENABLE_PPROF` enables the local pprof server on `localhost:6060`; do not enable it in production without an explicit operational need.
- Deployment manifests and RBAC changes belong under `deploy/` and should be reviewed together with the controller code that consumes them.

## Personal configuration

Read personal config at the start of any task that needs an assignee, email, or project key.
Canonical path: `~/.config/user.local.md` (tool-agnostic, global).
If the file does not exist, fall back to agent memory (`user-config`), then placeholders.
This repository does not define a `personalize` Make target; run `make personalize` from the Fleet Engineering `ocp-fleet-agentic-sdlc` checkout when setting up or updating the shared personal config.

## Tool Integrations

- GitHub CLI (`gh`) is not installed in this environment. Use the GitHub MCP server for GitHub operations; this repository is under the `stolostron` organization.
- Jira CLI is not configured. Use the Jira MCP server for Jira operations.
- Do not commit or push unless the user explicitly requests it.

## Fleet Engineering Skills

Fetch and apply the relevant skill when the task matches its domain.

| Skill | When to use |
|---|---|
| [bug-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/bug-specialist/SKILL.md) | Bug triage, reproduction steps, fix planning |
| [epic-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/epic-specialist/SKILL.md) | Multi-sprint epics with outcomes |
| [feature-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/feature-specialist/SKILL.md) | Large customer-facing capabilities |
| [initiative-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/initiative-specialist/SKILL.md) | Multi-team strategic programs |
| [jira-create](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/jira-create/SKILL.md) | Interactive issue creation with specialist delegation |
| [jira-qe-readiness](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/jira-qe-readiness/SKILL.md) | Check whether a Jira ticket has enough information for QE |
| [jira-report](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/jira-report/SKILL.md) | Jira portfolio reports and issue quality reviews |
| [jira-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/jira-specialist/SKILL.md) | General Jira triage, search, linking, and transitions |
| [jira-type-audit](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/jira-type-audit/SKILL.md) | Audit and correct Jira issue types across hierarchies |
| [outcome-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/outcome-specialist/SKILL.md) | Strategic outcomes tied to OKRs |
| [release-dod](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/release-dod/SKILL.md) | Release Definition of Done checklists |
| [risk-report](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/risk-report/SKILL.md) | Automated risk signal detection and status-report drafting |
| [risk-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/risk-specialist/SKILL.md) | Risk registers and mitigation planning |
| [spike-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/spike-specialist/SKILL.md) | Time-boxed research and proof of concepts |
| [story-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/story-specialist/SKILL.md) | User stories and acceptance criteria |
| [supportex-review](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/supportex-review/SKILL.md) | Review and approve SUPPORTEX requests |
| [task-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/task-specialist/SKILL.md) | Internal technical tasks |
| [ticket-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/ticket-specialist/SKILL.md) | Stakeholder request intake and triage |
| [backlog-grooming](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/backlog-grooming/SKILL.md) | Jira backlog grooming and readiness analysis |
| [breaking-changes](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/breaking-changes/SKILL.md) | Detect API, configuration, behavior, and integration breaking changes |
| [ci-triage](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/ci-triage/SKILL.md) | Diagnose failing CI checks on a pull request |
| [coderabbit-sync](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/coderabbit-sync/SKILL.md) | Maintain the Fleet reference CodeRabbit configuration |
| [cve-sustaining-handoff](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/cve-sustaining-handoff/SKILL.md) | Resolve or hand off CVE sustaining work |
| [cve-triage](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/cve-triage/SKILL.md) | Gather CVE evidence and VEX dispositions |
| [vulnerability-slack-report](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/vulnerability-slack-report/SKILL.md) | Post weekly overdue vulnerability summaries |
| [diagnosing-bugs](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/diagnosing-bugs/SKILL.md) | Structured debugging for unclear failures |
| [finish-work](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/finish-work/SKILL.md) | Commit, push, open a pull request, and update Jira |
| [github-org-access](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/github-org-access/SKILL.md) | Modify GitHub organization access configuration |
| [init-context-docs](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/init-context-docs/SKILL.md) | Assess and bootstrap repository context documentation |
| [opencode-setup](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/opencode-setup/SKILL.md) | Install and configure OpenCode |
| [org-repo-audit](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/org-repo-audit/SKILL.md) | Audit organization repositories for SDLC readiness |
| [pr-fix](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/pr-fix/SKILL.md) | Fix merge conflicts, CI failures, or review comments |
| [pr-hygiene](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/pr-hygiene/SKILL.md) | Manage stale pull request lifecycle |
| [pr-review](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/pr-review/SKILL.md) | Review GitHub pull requests with inline findings |
| [pr-review-detailed](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/pr-review-detailed/SKILL.md) | Run layered checklist-based code analysis |
| [pr-review-fix](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/pr-review-fix/SKILL.md) | Iteratively review and fix local changes before commit |
| [release-notes](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/release-notes/SKILL.md) | Generate categorized release notes |
| [renovate-prs](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/renovate-prs/SKILL.md) | Manage Renovate and MintMaker dependency pull requests |
| [repo-content-audit](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/repo-content-audit/SKILL.md) | Find unlinked or orphaned repository content |
| [repo-setup](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/repo-setup/SKILL.md) | Onboard a repository to the Fleet Engineering Agentic SDLC |
| [rhacm-addon-wizard](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/rhacm-addon-wizard/SKILL.md) | Guide RHACM add-on development |
| [scored-code-review](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/scored-code-review/SKILL.md) | Deprecated; superseded by `pr-review-detailed` |
| [session-summary](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/session-summary/SKILL.md) | Summarize session work against Jira and GitHub |
| [start-work](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/start-work/SKILL.md) | Create a Jira sub-task for a work session |
| [test-coverage-gap](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/test-coverage-gap/SKILL.md) | Analyze coverage gaps and suggest tests |
| [f2f-daily-summary](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/f2f-daily-summary/SKILL.md) | Capture daily F2F notes as Jira sub-tasks |
| [f2f-epic-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/f2f-epic-specialist/SKILL.md) | Create and manage F2F meeting epics |
| [presentation-task](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/presentation-task/SKILL.md) | Log delivered presentations as Jira work |
| [scrum-status](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/scrum-status/SKILL.md) | Capture scrum bullets and generate status reports |
