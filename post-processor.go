package main

import (
	"github.com/hashicorp/packer/packer"
	"github.com/hashicorp/packer/packer/plugin"
	"github.com/hashicorp/packer/common"
	"github.com/hashicorp/packer/helper/config"
	"os"
	"strings"
	"fmt"
	"net/http"
	"bytes"
	"errors"
)

const TeamcityVersionEnvVar = "TEAMCITY_VERSION"

var AmazonBuilderIds = []string{
	"mitchellh.amazonebs",
	"mitchellh.amazon.ebssurrogate",
	"mitchellh.amazon.instance",
	"mitchellh.amazon.chroot",
}

func main() {
	server, err := plugin.Server()
	if err != nil {
		panic(err)
	}
	server.RegisterPostProcessor(new(PostProcessor))
	server.Serve()
}

type PostProcessor struct {
	config Config
}

type Config struct {
	common.PackerConfig `mapstructure:",squash"`

	TeamCityUrl     string `mapstructure:"teamcity_url"`
	Username        string `mapstructure:"username"`
	Password        string `mapstructure:"password"`
	ProjectId       string `mapstructure:"project_id"`
	CustomImageName string `mapstructure:"custom_image_name"`
	AgentName       string `mapstructure:"agent_name"`
}

func (p *PostProcessor) Configure(raws ...interface{}) error {
	err := config.Decode(&p.config, nil, raws...)
	if err != nil {
		return err
	}

	errs := new(packer.MultiError)
	if p.config.TeamCityUrl != "" {
		if p.config.Username == "" {
			errs = packer.MultiErrorAppend(errs, fmt.Errorf("username is required"))
		}
		if p.config.Password == "" {
			errs = packer.MultiErrorAppend(errs, fmt.Errorf("password is required"))
		}
		if p.config.ProjectId == "" {
			errs = packer.MultiErrorAppend(errs, fmt.Errorf("project_id is required"))
		}
		if p.config.CustomImageName == "" {
			errs = packer.MultiErrorAppend(errs, fmt.Errorf("custom_image_name is required"))
		}
		if p.config.AgentName == "" {
			errs = packer.MultiErrorAppend(errs, fmt.Errorf("agent_name is required"))
		}
	}

	if len(errs.Errors) > 0 {
		return errs
	}

	return nil
}

func (p *PostProcessor) PostProcess(ui packer.Ui, artifact packer.Artifact) (packer.Artifact, bool, error) {
	if os.Getenv(TeamcityVersionEnvVar) != "" {
		if contains(AmazonBuilderIds, artifact.BuilderId()) {
			s := strings.Split(artifact.Id(), ":")
			region, ami := s[0], s[1]
			ui.Message(fmt.Sprintf("##teamcity[setParameter name='packer.artifact.%v.aws.region' value='%v']", p.config.PackerBuildName, region))
			ui.Message(fmt.Sprintf("##teamcity[setParameter name='packer.artifact.%v.aws.ami' value='%v']", p.config.PackerBuildName, ami))
		} else {
			ui.Message(fmt.Sprintf("##teamcity[setParameter name='packer.artifact.%v.id' value='%v']", p.config.PackerBuildName, artifact.Id()))
			ui.Message(fmt.Sprintf("##teamcity[setParameter name='packer.artifact.last.id' value='%v']", artifact.Id()))
		}
	}

	if p.config.TeamCityUrl != "" {
		url := fmt.Sprintf(
			"%v/httpAuth/app/rest/projects/id:%v/projectFeatures/type:CloudImage,property(name:source-id,value:%v)/properties/sourceVmName",
			strings.TrimRight(p.config.TeamCityUrl, "/"),
			p.config.ProjectId,
			p.config.CustomImageName,
		)
		body := bytes.NewBufferString(p.config.AgentName)

		c := &http.Client{}
		req, err := http.NewRequest("PUT", url, body)
		if err != nil {
			return artifact, true, err
		}
		req.Header.Add("Content-Type", "text/plain")
		req.SetBasicAuth(p.config.Username, p.config.Password)

		resp, err := c.Do(req)
		if err != nil {
			return artifact, true, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return artifact, true, errors.New(fmt.Sprintf("Error updating a cloud profile: %v", resp.Status))
		}

		ui.Message(fmt.Sprintf("Cloud agent image '%v' is switched to image '%v'", p.config.CustomImageName, p.config.AgentName))
	}

	return artifact, true, nil
}

func contains(slice []string, value string) bool {
	for _, element := range slice {
		if element == value {
			return true
		}
	}
	return false
}
