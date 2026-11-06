// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package main defines the GCP infrastructure for Alertmanager load testing.
package main

import (
	"fmt"
	"strconv"

	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/compute"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	defaultInstanceCount = 1
	maxInstanceCount     = 3
	defaultMachineType   = "e2-medium"
)

func main() {
	pulumi.Run(run)
}

func run(ctx *pulumi.Context) error {
	cfg := config.New(ctx, "")

	instanceCount, err := parseInstanceCount(cfg.Get("instanceCount"))
	if err != nil {
		return err
	}

	machineType := cfg.Get("machineType")
	if machineType == "" {
		machineType = defaultMachineType
	}

	zone := config.Require(ctx, "gcp:zone")

	instanceNames := make(pulumi.StringArray, 0, instanceCount)
	instanceIDs := make(pulumi.IDArray, 0, instanceCount)
	privateIPs := make(pulumi.StringArray, 0, instanceCount)

	for i := 1; i <= instanceCount; i++ {
		name := fmt.Sprintf("alertmanager-loadtest-%d", i)
		instance, err := compute.NewInstance(ctx, name, &compute.InstanceArgs{
			Name:        pulumi.String(name),
			MachineType: pulumi.String(machineType),
			Zone:        pulumi.String(zone),
			BootDisk: &compute.InstanceBootDiskArgs{
				InitializeParams: &compute.InstanceBootDiskInitializeParamsArgs{
					Image: pulumi.String("debian-cloud/debian-12"),
					Size:  pulumi.Int(20),
				},
			},
			NetworkInterfaces: compute.InstanceNetworkInterfaceArray{
				&compute.InstanceNetworkInterfaceArgs{
					Network: pulumi.String("default"),
				},
			},
		})
		if err != nil {
			return fmt.Errorf("create instance %s: %w", name, err)
		}

		privateIP := instance.NetworkInterfaces.ApplyT(func(interfaces []compute.InstanceNetworkInterface) (string, error) {
			if len(interfaces) != 1 || interfaces[0].NetworkIp == nil {
				return "", fmt.Errorf("instance %s does not have one private IP address", name)
			}
			return *interfaces[0].NetworkIp, nil
		}).(pulumi.StringOutput)

		instanceNames = append(instanceNames, instance.Name)
		instanceIDs = append(instanceIDs, instance.ID())
		privateIPs = append(privateIPs, privateIP)
	}

	ctx.Export("instanceNames", instanceNames)
	ctx.Export("instanceIds", instanceIDs)
	ctx.Export("privateIpAddresses", privateIPs)
	return nil
}

func parseInstanceCount(value string) (int, error) {
	if value == "" {
		return defaultInstanceCount, nil
	}

	count, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("instanceCount must be an integer: %w", err)
	}
	if count < 1 || count > maxInstanceCount {
		return 0, fmt.Errorf("instanceCount must be between 1 and %d", maxInstanceCount)
	}
	return count, nil
}
