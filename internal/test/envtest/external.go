// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package envtest

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func CRDDirectoryPaths() []string {
	return []string{
		filepath.Join(
			getModulePath("sigs.k8s.io/cluster-api"),
			"config",
			"crd",
			"bases",
		),
		filepath.Join(
			rootModulePath(),
			"charts",
			"cluster-api-ipam-provider-nutanix",
			"crds",
		),
	}
}

func rootModulePath() string {
	return getModulePath("")
}

func getModulePath(moduleName string) string {
	goArgs := []string{"list", "-m", "-f", "{{ .Dir }}"}
	if moduleName != "" {
		goArgs = append(goArgs, moduleName)
	}
	cmd := exec.Command("go", goArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// We include the combined output because the error is usually
		// an exit code, which does not explain why the command failed.
		panic(
			fmt.Sprintf("cmd.Dir=%q, cmd.Env=%q, cmd.Args=%q, err=%q, output=%q",
				cmd.Dir,
				cmd.Env,
				cmd.Args,
				err,
				out),
		)
	}
	return strings.TrimSpace(string(out))
}
