/*
Copyright 2021 The Knative Authors

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

package install

import (
	"context"
	"fmt"
	"os"
	"time"

	cmdtpl "github.com/k14s/ytt/pkg/cmd/template"
	"github.com/k14s/ytt/pkg/cmd/ui"
	"github.com/k14s/ytt/pkg/files"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	clientset "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc" // from https://github.com/kubernetes/client-go/issues/345
	"k8s.io/client-go/tools/clientcmd"
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
			clients, err := p.NewKubeClient()
			if err != nil {
				return fmt.Errorf("cannot get source cluster kube config, please use --kubeconfig or export environment variable KUBECONFIG to set\n")
			}

			restConfig, err := p.RestConfig()
			if err != nil {
				return err
			}

			rootPath, err := os.Getwd()
			if err != nil {
				return err
			}

			URL, err := common.GetOperatorURL(installFlags.Version)
			if err != nil {
				return err
			}

			yamlTemplateString, err := common.DownloadFile(URL)
			if err != nil {
				return err
			}

			yamlTemplateData := []byte(yamlTemplateString)

			operatorOverlay := rootPath + "/overlay/operator.yaml"
			overlayContent, err := common.ReadFile(operatorOverlay)
			if err != nil {
				return err
			}
			yamlOverlayData := []byte(overlayContent)

			if installFlags.Namespace != "default" {
				// Create the namespace if it is not available
				_, err := clients.CoreV1().Namespaces().Get(context.TODO(), installFlags.Namespace, metav1.GetOptions{})
				if apierrors.IsNotFound(err) {
					ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: installFlags.Namespace,
						Labels: map[string]string{"istio-injection": "enabled"}}}
					clients.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
				} else if err != nil {
					return err
				}
			}

			// Create a temporary directory to host all the temporary files
			t := time.Now()
			tUnixMilli := int64(time.Nanosecond) * t.UnixNano() / int64(time.Millisecond)
			tempDir := fmt.Sprintf("/tmp/temp-%d", tUnixMilli)
			if err = os.Mkdir(tempDir, 0755); err != nil {
				return err
			}

			templatePath := fmt.Sprintf("%s/%s", tempDir, "tpl.yml")
			overlayPath := fmt.Sprintf("%s/%s", tempDir, "overlay.yml")
			valuesPath := fmt.Sprintf("%s/%s", tempDir, "values.yml")

			yamlValuesContent := fmt.Sprintf("#@data/values\n---\nnamespace: %s", installFlags.Namespace)
			yamlValuesData := []byte(yamlValuesContent)
			filesToProcess := []*files.File{
				files.MustNewFileFromSource(files.NewBytesSource(templatePath, yamlTemplateData)),
				files.MustNewFileFromSource(files.NewBytesSource(overlayPath, yamlOverlayData)),
				files.MustNewFileFromSource(files.NewBytesSource(valuesPath, yamlValuesData)),
			}

			ui := ui.NewTTY(false)
			opts := cmdtpl.NewOptions()
			out := opts.RunWithFiles(cmdtpl.Input{Files: filesToProcess}, ui)
			finalFile := out.Files[0]
			finalContent := string(finalFile.Bytes())

			finalPath := finalFile.RelativePath()
			if err = common.WriteFile(finalPath, finalContent); err != nil {
				return err
			}

			// Apply the content of the YAML file
			if err = common.ApplyFile(finalPath, restConfig); err != nil {
				return err
			}

			// Remove all the files under the temporary directory
			if err = os.RemoveAll(tempDir); err != nil {
				return err
			}

			return nil
		},
	}

	installCmd.Flags().StringVarP(&installFlags.Namespace, "namespace", "n", "default", "The namespace of the Knative Operator or the Knative component")
	installCmd.Flags().StringVarP(&installFlags.Component, "component", "c", "", "The name of the Knative Component to install")
	installCmd.Flags().StringVarP(&installFlags.Version, "version", "v", "latest", "The version of the the Knative Operator or the Knative component")
	installCmd.Flags().StringVar(&installFlags.IstioNamespace, "istio-namespace", "", "The namespace of istio")

	return installCmd
}

func getClients(kubeConfig, namespace string) (*kubernetes.Clientset, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, err
	}
	clientSet, err := clientset.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return clientSet, nil
}
