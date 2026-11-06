# Alertmanager load testing infrastructure on GCP

This Pulumi project creates private Compute Engine VMs for Alertmanager load testing. It does not install Alertmanager or a load generator. The VMs do not have public IP addresses.

This project does not configure SSH access or firewall rules. Existing firewall rules and authentication settings can still allow SSH from private networks or through Identity-Aware Proxy.

## Configuration

| Key | Default | Description |
| --- | --- | --- |
| `gcp:project` | None | GCP project ID. Required. |
| `gcp:zone` | None | GCP zone. Required. |
| `instanceCount` | `1` | Number of VMs. Valid values are `1` through `3`. |
| `machineType` | `e2-medium` | Compute Engine machine type. |

Each VM uses Debian 12, a 20 GB boot disk, and the default VPC.

## Local checks

Install the Go version from `go.mod`. Run these commands from this directory:

```shell
go test ./...
go build ./...
```

These checks do not require GCP access.

## Deploy

You need:

- The Pulumi CLI.
- The Google Cloud CLI.
- A GCP project with billing and the Compute Engine API enabled.
- Permission to create Compute Engine instances in the project.

Authenticate and create a stack:

```shell
gcloud auth application-default login
pulumi login --local
pulumi stack init loadtest
pulumi config set gcp:project YOUR_PROJECT_ID
pulumi config set gcp:zone YOUR_ZONE
```

The local backend stores state on this workstation. Use a shared backend if more than one person must manage the stack.

Set an optional machine type:

```shell
pulumi config set machineType e2-standard-4
```

The committed `loadtest` configuration sets `instanceCount` to `3`. The first deployment creates three VMs.

Preview and create the resources:

```shell
pulumi preview
pulumi up
```

Show the instance names, IDs, and private IP addresses:

```shell
pulumi stack output
```

## GitHub Actions deployment

The repository contains a deployment workflow at `.github/workflows/deploy-loadtest.yml.disabled`. GitHub Actions ignores the `.disabled` suffix.

Before you enable the workflow:

1. Create a private GCS bucket for Pulumi state.
2. Configure Workload Identity Federation and a deployment service account. Grant the service account permission to manage Compute Engine instances and objects in the state bucket. Grant permission to use the VM service account if required.
3. Create the `alertmanager-loadtest` GitHub environment. Add required reviewers to control deployments.
4. Add these secrets to the environment:
   - `GCP_WORKLOAD_IDENTITY_PROVIDER`
   - `GCP_SERVICE_ACCOUNT`
   - `PULUMI_CONFIG_PASSPHRASE`
5. Add these variables to the environment:
   - `GCP_PROJECT`
   - `GCP_ZONE`
   - `PULUMI_BACKEND_URL`, with the format `gs://BUCKET_NAME`

Enable the workflow:

```shell
mv .github/workflows/deploy-loadtest.yml.disabled .github/workflows/deploy-loadtest.yml
```

Commit and push the renamed file. After you enable the workflow, a push to `main` triggers deployment when it changes `test/load/gcp/` or the deployment workflow.

The workflow uses the GCS backend and the `loadtest` stack. It reads the VM count from Pulumi configuration. The committed `loadtest` configuration uses three VMs. Migrate existing state before switching a local deployment to this backend.

## Destroy

Remove all resources managed by the stack:

```shell
pulumi destroy
```
