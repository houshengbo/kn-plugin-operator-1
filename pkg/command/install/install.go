// Copyright 2021 The Knative Authors
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

package install

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc" // from https://github.com/kubernetes/client-go/issues/345
	"knative.dev/kn-plugin-operator/pkg"
	"knative.dev/kn-plugin-operator/pkg/command/common"
)

type installCmdFlags struct {
	Component      string
	IstioNamespace string
	Namespace      string
	KubeConfig     string
	Version        string
}

func (flags *installCmdFlags) fill_defaults() {
	if flags.Version == "" {
		flags.Version = "latest"
	}

	if flags.IstioNamespace == "" && strings.EqualFold(flags.Component, common.ServingComponent) {
		flags.IstioNamespace = common.DefaultIstioNamespace
	}

	if flags.Namespace == "" {
		if strings.EqualFold(flags.Component, common.ServingComponent) {
			flags.Namespace = common.DefaultKnativeServingNamespace
		} else if strings.EqualFold(flags.Component, common.EventingComponent) {
			flags.Namespace = common.DefaultKnativeEventingNamespace
		} else if flags.Component == "" {
			flags.Namespace = common.DefaultNamespace
		}
	}
}

var (
	installFlags installCmdFlags
)

// installCmd represents the install commands for the operation
func NewInstallCommand(p *pkg.OperatorParams) *cobra.Command {
	var installCmd = &cobra.Command{
		Use:   "install",
		Short: "Install Knative Operator or Knative components",
		Example: `
  # Install Knative Serving under the namespace knative-serving
  kn operation install -c serving --namespace knative-serving`,

		RunE: func(cmd *cobra.Command, args []string) error {
			// Fill in the default values for the empty fields
			installFlags.fill_defaults()
			p.KubeCfgPath = installFlags.KubeConfig

			rootPath, err := os.Getwd()
			if err != nil {
				return err
			}

			if installFlags.Component != "" {
				// Install serving or eventing
				err = installKnativeComponent(installFlags, rootPath, p)
				if err != nil {
					return err
				}
			} else {
				// Install the Knative Operator
				err = installOperator(installFlags, rootPath, p)
				if err != nil {
					return err
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Knative operator of the '%s' version was created in the namespace '%s'.\n", installFlags.Version, installFlags.Namespace)
			return nil
		},
	}

	installCmd.Flags().StringVar(&installFlags.KubeConfig, "kubeconfig", "", "The kubeconfig of the Knative resources (default is KUBECONFIG from environment variable)")
	installCmd.Flags().StringVarP(&installFlags.Namespace, "namespace", "n", "", "The namespace of the Knative Operator or the Knative component")
	installCmd.Flags().StringVarP(&installFlags.Component, "component", "c", "", "The name of the Knative Component to install")
	installCmd.Flags().StringVarP(&installFlags.Version, "version", "v", "latest", "The version of the the Knative Operator or the Knative component")
	installCmd.Flags().StringVar(&installFlags.IstioNamespace, "istio-namespace", "", "The namespace of istio")

	return installCmd
}

func getOperatorURL(version string) (string, error) {
	versionSanitized := strings.ToLower(version)
	URL := "https://github.com/knative/operator/releases/latest/download/operator.yaml"
	if version != "latest" {
		if !strings.HasPrefix(version, "v") {
			versionSanitized = fmt.Sprintf("v%s", versionSanitized)
		}
		validity, major := common.GetMajor(versionSanitized)
		if !validity {
			return "", fmt.Errorf("%v is not a semantic version", version)
		}
		prefix := ""
		if semver.Compare(major, "v0") == 1 {
			prefix = "knative-"
		}
		URL = fmt.Sprintf("https://github.com/knative/operator/releases/download/%s%s/operator.yaml", prefix, versionSanitized)
	}
	return URL, nil
}

func getOverlayYamlContent(installFlags installCmdFlags, rootPath string) string {
	path := ""
	if strings.EqualFold(installFlags.Component, common.ServingComponent) {
		path = rootPath + "/overlay/ks.yaml"
		if installFlags.IstioNamespace != common.DefaultIstioNamespace {
			path = rootPath + "/overlay/ks_istio_ns.yaml"
		}
	} else if strings.EqualFold(installFlags.Component, common.EventingComponent) {
		path = rootPath + "/overlay/ke.yaml"
	} else if installFlags.Component == "" {
		path = rootPath + "/overlay/operator.yaml"
	}

	if path == "" {
		return ""
	}
	overlayContent, _ := common.ReadFile(path)
	return overlayContent
}

func getYamlValuesContent(installFlags installCmdFlags) string {
	content := ""
	if strings.EqualFold(installFlags.Component, common.ServingComponent) {
		content = fmt.Sprintf("#@data/values\n---\nname: %s\nnamespace: %s\nversion: '%s'",
			common.DefaultKnativeServingNamespace, installFlags.Namespace, installFlags.Version)
		if installFlags.IstioNamespace != common.DefaultIstioNamespace {
			myslice := []string{content, fmt.Sprintf("local_gateway_value: knative-local-gateway.%s.svc.cluster.local", installFlags.IstioNamespace)}
			content = strings.Join(myslice, "\n")
		}
	} else if strings.EqualFold(installFlags.Component, common.EventingComponent) {
		content = fmt.Sprintf("#@data/values\n---\nname: %s\nnamespace: %s\nversion: '%s'",
			common.DefaultKnativeEventingNamespace, installFlags.Namespace, installFlags.Version)
	} else if installFlags.Component == "" {
		content = fmt.Sprintf("#@data/values\n---\nnamespace: %s", installFlags.Namespace)
	}
	return content
}

func installKnativeComponent(installFlags installCmdFlags, rootPath string, p *pkg.OperatorParams) error {
	client, err := p.NewKubeClient()
	if err != nil {
		return fmt.Errorf("cannot get source cluster kube config, please use --kubeconfig or export environment variable KUBECONFIG to set\n")
	}

	deploy := common.Deployment{
		Client: client,
	}

	// Check if the knative operator is installed
	if exists, err := deploy.CheckIfOperatorInstalled(); err != nil {
		return err
	} else if !exists {
		operatorInstallFlags := installCmdFlags{
			Namespace: "default",
			Version:   "latest",
		}
		installOperator(operatorInstallFlags, rootPath, p)
	}

	err = createNamspaceIfNecessary(installFlags.Namespace, p)
	if err != nil {
		return err
	}

	// Generate the CR template
	yamlTemplateString, err := common.GenerateOperatorCRString(installFlags.Component, installFlags.Namespace, p)
	if err != nil {
		return err
	}

	return applyOverlayValuesOnTemplate(yamlTemplateString, installFlags, rootPath, p)
}

func installOperator(installFlags installCmdFlags, rootPath string, p *pkg.OperatorParams) error {
	err := createNamspaceIfNecessary(installFlags.Namespace, p)
	if err != nil {
		return err
	}

	URL, err := getOperatorURL(installFlags.Version)
	if err != nil {
		return err
	}

	// Generate the CR template by downloading the operator yaml
	yamlTemplateString, err := common.DownloadFile(URL)
	if err != nil {
		return err
	}

	return applyOverlayValuesOnTemplate(yamlTemplateString, installFlags, rootPath, p)
}

func createNamspaceIfNecessary(namespace string, p *pkg.OperatorParams) error {
	client, err := p.NewKubeClient()
	if err != nil {
		return fmt.Errorf("cannot get source cluster kube config, please use --kubeconfig or export environment variable KUBECONFIG to set\n")
	}

	ns := common.Namespace{
		Client:    client,
		Component: namespace,
	}
	if err = ns.CreateNamespace(namespace); err != nil {
		return err
	}
	return nil
}

func applyOverlayValuesOnTemplate(yamlTemplateString string, installFlags installCmdFlags, rootPath string, p *pkg.OperatorParams) error {
	overlayContent := getOverlayYamlContent(installFlags, rootPath)
	yamlValuesContent := getYamlValuesContent(installFlags)

	if err := common.ApplyManifests(yamlTemplateString, overlayContent, yamlValuesContent, p); err != nil {
		return err
	}
	return nil
}
