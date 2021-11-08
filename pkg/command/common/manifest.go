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

package common

import (
	"fmt"
	"strings"

	mfc "github.com/manifestival/client-go-client"
	"golang.org/x/mod/semver"
	"k8s.io/client-go/rest"
)

// ApplyFile applies the content of the yaml file against the Kubernetes cluster
func ApplyFile(path string, restConfig *rest.Config) error {
	manifest, err := mfc.NewManifest(path, restConfig)
	if err != nil {
		return err
	}

	if err := manifest.Apply(); err != nil {
		return err
	}

	return nil
}

func GetOperatorURL(version string) (string, error) {
	URL := "https://github.com/knative/operator/releases/latest/download/operator.yaml"
	if version != "latest" {
		versionSanitized := version
		if !strings.HasPrefix(version, "v") {
			versionSanitized = fmt.Sprintf("v%s", versionSanitized)
		}
		validity, major := GetMajor(versionSanitized)
		if !validity {
			return "", fmt.Errorf("%v is not a semantic version", version)
		}
		prefix := ""
		if semver.Compare(major, "v0") == 1 {
			prefix = "knative-"
		}
		URL = fmt.Sprintf("https://github.com/knative/operator/releases/download/%s%s/operator.yaml", prefix, version)
	}
	return URL, nil
}
