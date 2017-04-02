package cloudcfg

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/quilt/quilt/db"
)

const (
	quiltImage = "mmussomele/quilt:latest"
)

// Ubuntu generates a cloud config file for the Ubuntu operating system with the
// corresponding `version`.
func Ubuntu(keys []string, version string, role db.Role) string {
	t := template.Must(template.New("cloudConfig").Parse(cfgTemplate))

	var cloudConfigBytes bytes.Buffer
	err := t.Execute(&cloudConfigBytes, struct {
		QuiltImage    string
		UbuntuVersion string
		SSHKeys       string
		Role          string
	}{
		QuiltImage:    quiltImage,
		UbuntuVersion: version,
		SSHKeys:       strings.Join(keys, "\n"),
		Role:          string(role),
	})
	if err != nil {
		panic(err)
	}

	return cloudConfigBytes.String()
}
