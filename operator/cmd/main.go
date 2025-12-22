// /*
// Copyright 2024 The Grove Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// */

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	configv1alpha1 "github.com/ai-dynamo/grove/operator/api/config/v1alpha1"
	grovecorev1alpha1 "github.com/ai-dynamo/grove/operator/api/core/v1alpha1"
	groveopts "github.com/ai-dynamo/grove/operator/cmd/opts"
	grovectrl "github.com/ai-dynamo/grove/operator/internal/controller"
	"github.com/ai-dynamo/grove/operator/internal/controller/cert"
	grovelogger "github.com/ai-dynamo/grove/operator/internal/logger"
	groveversion "github.com/ai-dynamo/grove/operator/internal/version"

	"github.com/spf13/pflag"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	logger = ctrl.Log.WithName("grove-setup")
)

func main() {
	ctrl.SetLogger(grovelogger.MustNewLogger(false, configv1alpha1.InfoLevel, configv1alpha1.LogFormatJSON))

	fs := pflag.CommandLine
	groveversion.AddFlags(fs)
	cliOpts := groveopts.NewCLIOptions(fs)

	// parse and print command line flags
	pflag.Parse()
	groveversion.PrintVersionAndExitIfRequested()

	logger.Info("Starting grove operator", "version", groveversion.Get())
	printFlags()

	operatorCfg, err := initializeOperatorConfig(cliOpts)
	if err != nil {
		logger.Error(err, "failed to initialize operator configuration")
		os.Exit(1)
	}

	mgr, err := grovectrl.CreateManager(operatorCfg)
	if err != nil {
		logger.Error(err, "failed to create grove controller manager")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()

	if err = ensureClusterTopology(ctx, mgr.GetClient(), operatorCfg.ClusterTopology); err != nil {
		logger.Error(err, "cannot create/update cluster topology, operator cannot start")
		os.Exit(1)
	}

	webhookCertsReadyCh := make(chan struct{})
	if err = cert.ManageWebhookCerts(mgr, operatorCfg.Server.Webhooks.ServerCertDir, operatorCfg.Authorizer.Enabled, webhookCertsReadyCh); err != nil {
		logger.Error(err, "failed to setup cert rotation")
		os.Exit(1)
	}

	if err = grovectrl.SetupHealthAndReadinessEndpoints(mgr, webhookCertsReadyCh); err != nil {
		logger.Error(err, "failed to set up health and readiness for grove controller manager")
		os.Exit(1)
	}

	// Certificates need to be generated before the webhooks are started, which can only happen once the manager is started.
	// Block while generating the certificates, and then start the webhooks.
	go func() {
		if err = grovectrl.RegisterControllersAndWebhooks(mgr, logger, operatorCfg, webhookCertsReadyCh); err != nil {
			logger.Error(err, "failed to initialize grove controller manager")
			os.Exit(1)
		}
	}()

	logger.Info("Starting manager")
	if err = mgr.Start(ctx); err != nil {
		logger.Error(err, "Error running manager")
		os.Exit(1)
	}
}

func initializeOperatorConfig(cliOpts *groveopts.CLIOptions) (*configv1alpha1.OperatorConfiguration, error) {
	// complete and validate operator configuration
	if err := cliOpts.Complete(); err != nil {
		return nil, err
	}
	if err := cliOpts.Validate(); err != nil {
		return nil, err
	}
	return cliOpts.Config, nil
}

func printFlags() {
	var flagKVs []any
	flag.VisitAll(func(f *flag.Flag) {
		flagKVs = append(flagKVs, f.Name, f.Value.String())
	})
	logger.Info("Running with flags", flagKVs...)
}

func ensureClusterTopology(ctx context.Context, client client.Client, config configv1alpha1.ClusterTopologyConfiguration) error {
	if !config.Enabled {
		return nil
	}

	if config.Name == "" {
		return errors.New("ClusterTopology name is required when enabled")
	}

	if len(config.Levels) == 0 {
		return errors.New("ClusterTopology levels are required when enabled")
	}

	topology := &grovecorev1alpha1.ClusterTopology{
		ObjectMeta: metav1.ObjectMeta{
			Name: config.Name,
		},
		Spec: grovecorev1alpha1.ClusterTopologySpec{
			Levels: convertTopologyLevels(config.Levels),
		},
	}

	err := client.Create(ctx, topology)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			existing := &grovecorev1alpha1.ClusterTopology{}
			if err := client.Get(ctx, types.NamespacedName{Name: config.Name}, existing); err != nil {
				return fmt.Errorf("failed to get existing ClusterTopology: %w", err)
			}
			existing.Spec = topology.Spec
			if err := client.Update(ctx, existing); err != nil {
				return fmt.Errorf("failed to update ClusterTopology: %w", err)
			}
			logger.Info("cluster topology updated successfully", "name", config.Name)
			return nil
		}
		return fmt.Errorf("failed to create ClusterTopology: %w", err)
	}

	logger.Info("cluster topology created successfully", "name", config.Name)
	return nil
}

func convertTopologyLevels(levels []configv1alpha1.TopologyLevel) []grovecorev1alpha1.TopologyLevel {
	result := make([]grovecorev1alpha1.TopologyLevel, len(levels))
	for i, level := range levels {
		result[i] = grovecorev1alpha1.TopologyLevel{
			Domain: grovecorev1alpha1.TopologyDomain(level.Domain),
			Key:    level.Key,
		}
	}
	return result
}
