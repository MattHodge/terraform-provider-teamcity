package teamcity

import (
	"bytes"
	"fmt"
	"strings"

	api "github.com/cvbarros/go-teamcity-sdk/pkg/teamcity"
	"github.com/hashicorp/terraform/helper/hashcode"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
)

func resourceBuildConfiguration() *schema.Resource {
	return &schema.Resource{
		Create: resourceBuildConfigurationCreate,
		Read:   resourceBuildConfigurationRead,
		Update: resourceBuildConfigurationUpdate,
		Delete: resourceBuildConfigurationDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"project_id": {
				Type:     schema.TypeString,
				Required: true,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"vcs_root": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeString,
							Required: true,
						},
						"checkout_rules": {
							Type:     schema.TypeList,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
					},
				},
				Set: vcsRootHash,
			},
			"step": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"type": {
							Type:         schema.TypeString,
							Required:     true,
							ValidateFunc: validation.StringInSlice([]string{"powershell", "cmd_line"}, false),
						},
						"name": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},
						"file": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"args": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"code": {
							Type:     schema.TypeString,
							Optional: true,
						},
					},
				},
				Set: stepSetHash,
			},
			"env_params": {
				Type:     schema.TypeMap,
				Optional: true,
			},
			"config_params": {
				Type:     schema.TypeMap,
				Optional: true,
			},
			"sys_params": {
				Type:     schema.TypeMap,
				Optional: true,
			},
		},
	}
}

func resourceBuildConfigurationCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*api.Client)
	var projectID, name string

	if v, ok := d.GetOk("project_id"); ok {
		projectID = v.(string)
	}

	if v, ok := d.GetOk("name"); ok {
		name = v.(string)
	}

	bt, err := api.NewBuildType(projectID, name)
	if err != nil {
		return err
	}

	if v, ok := d.GetOk("description"); ok {
		bt.Description = v.(string)
	}

	bt.Parameters, err = expandParameterCollection(d)
	if err != nil {
		return err
	}

	created, err := client.BuildTypes.Create(projectID, bt)

	if err != nil {
		return err
	}

	d.MarkNewResource()
	d.SetId(created.ID)
	d.Partial(true)

	if v, ok := d.GetOk("vcs_root"); ok {
		vcs := v.(*schema.Set).List()
		for _, raw := range vcs {
			toAttach := buildVcsRootEntry(raw)

			err = client.BuildTypes.AttachVcsRootEntry(created.ID, toAttach)

			if err != nil {
				return err
			}
		}
		d.SetPartial("vcs_root")
	}

	if v, ok := d.GetOk("step"); ok {
		steps := v.(*schema.Set).List()
		for _, raw := range steps {

			newStep, err := expandBuildStep(raw)
			if err != nil {
				return err
			}

			_, err = client.BuildTypes.AddStep(created.ID, newStep)
			if err != nil {
				return err
			}
		}
		d.SetPartial("step")
	}

	d.Partial(false)

	return resourceBuildConfigurationRead(d, meta)
}

func resourceBuildConfigurationRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*api.Client)

	dt, err := getBuildConfiguration(client, d.Id())
	if err != nil {
		return err
	}

	if err := d.Set("name", dt.Name); err != nil {
		return err
	}

	if err := d.Set("description", dt.Description); err != nil {
		return err
	}

	if err := d.Set("project_id", dt.ProjectID); err != nil {
		return err
	}

	err = flattenParameterCollection(d, dt.Parameters)
	if err != nil {
		return err
	}

	vcsRoots := dt.VcsRootEntries

	if vcsRoots != nil && len(vcsRoots) > 0 {
		var vcsToSave []map[string]interface{}
		for _, el := range vcsRoots {
			m := make(map[string]interface{})
			m["id"] = el.ID
			m["checkout_rules"] = strings.Split(el.CheckoutRules, "\\n")
			vcsToSave = append(vcsToSave, m)
		}

		if err := d.Set("vcs_root", vcsToSave); err != nil {
			return err
		}
	}

	steps, err := client.BuildTypes.GetSteps(d.Id())
	if err != nil {
		return err
	}
	if steps != nil && len(steps) > 0 {
		var stepsToSave []map[string]interface{}
		for _, el := range steps {
			l, err := flattenBuildStep(el)
			if err != nil {
				return err
			}
			stepsToSave = append(stepsToSave, l)
		}

		if err := d.Set("step", stepsToSave); err != nil {
			return err
		}
	}

	return nil
}

func resourceBuildConfigurationUpdate(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func resourceBuildConfigurationDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*api.Client)
	return client.BuildTypes.Delete(d.Id())
}

func getBuildConfiguration(c *api.Client, id string) (*api.BuildType, error) {
	dt, err := c.BuildTypes.GetByID(id)
	if err != nil {
		return nil, err
	}

	return dt, nil
}

var stepTypeMap = map[string]string{
	api.StepTypePowershell:  "powershell",
	api.StepTypeCommandLine: "cmd_line",
}

func flattenParameterCollection(d *schema.ResourceData, params *api.Parameters) error {
	var configParams, sysParams, envParams = flattenParameters(params)

	if len(envParams) > 0 {
		if err := d.Set("env_params", envParams); err != nil {
			return err
		}
	}
	if len(sysParams) > 0 {
		if err := d.Set("sys_params", sysParams); err != nil {
			return err
		}
	}
	if len(configParams) > 0 {
		if err := d.Set("config_params", configParams); err != nil {
			return err
		}
	}
	return nil
}

func expandParameterCollection(d *schema.ResourceData) (*api.Parameters, error) {
	var config, system, env *api.Parameters
	if v, ok := d.GetOk("env_params"); ok {
		p, err := expandParameters(v.(map[string]interface{}), api.ParameterTypes.EnvironmentVariable)
		if err != nil {
			return nil, err
		}
		env = p
	}

	if v, ok := d.GetOk("sys_params"); ok {
		p, err := expandParameters(v.(map[string]interface{}), api.ParameterTypes.System)
		if err != nil {
			return nil, err
		}
		system = p
	}

	if v, ok := d.GetOk("config_params"); ok {
		p, err := expandParameters(v.(map[string]interface{}), api.ParameterTypes.Configuration)
		if err != nil {
			return nil, err
		}
		config = p
	}

	out := api.NewParametersEmpty()

	if config != nil {
		out = out.Concat(config)
	}
	if system != nil {
		out = out.Concat(system)
	}
	if env != nil {
		out = out.Concat(env)
	}
	return out, nil
}
func flattenParameters(dt *api.Parameters) (config map[string]string, sys map[string]string, env map[string]string) {
	env, sys, config = make(map[string]string), make(map[string]string), make(map[string]string)
	for _, p := range dt.Items {
		switch p.Type {
		case api.ParameterTypes.Configuration:
			config[p.Name] = p.Value
		case api.ParameterTypes.EnvironmentVariable:
			env[p.Name] = p.Value
		case api.ParameterTypes.System:
			sys[p.Name] = p.Value
		}
	}
	return config, sys, env
}

func expandParameters(raw map[string]interface{}, paramType string) (*api.Parameters, error) {
	out := api.NewParametersEmpty()
	for k, v := range raw {
		p, err := api.NewParameter(paramType, k, v.(string))
		if err != nil {
			return nil, err
		}
		out.AddOrReplaceParameter(p)
	}
	return out, nil
}

func flattenBuildStep(s api.Step) (map[string]interface{}, error) {
	mapType := stepTypeMap[s.Type()]

	switch mapType {
	case "powershell":
		return flattenBuildStepPowershell(s.(*api.StepPowershell)), nil
	case "cmd_line":
		return flattenBuildStepCmdLine(s.(*api.StepCommandLine)), nil
	default:
		return nil, fmt.Errorf("Build step type '%s' not supported", s.Type())
	}
}

func flattenBuildStepPowershell(s *api.StepPowershell) map[string]interface{} {
	m := make(map[string]interface{})
	if s.ScriptFile != "" {
		m["file"] = s.ScriptFile
		if s.ScriptArgs != "" {
			m["args"] = s.ScriptArgs
		}
	}
	if s.Code != "" {
		m["code"] = s.Code
	}
	if s.Name() != "" {
		m["name"] = s.Name()
	}
	m["type"] = "powershell"

	return m
}

func flattenBuildStepCmdLine(s *api.StepCommandLine) map[string]interface{} {
	m := make(map[string]interface{})
	if s.CommandExecutable != "" {
		m["file"] = s.CommandExecutable
		if s.CommandParameters != "" {
			m["args"] = s.CommandParameters
		}
	}
	if s.CustomScript != "" {
		m["code"] = s.CustomScript
	}
	if s.Name() != "" {
		m["name"] = s.Name()
	}
	m["type"] = "cmd_line"

	return m
}

func expandBuildStep(raw interface{}) (api.Step, error) {
	localStep := raw.(map[string]interface{})

	t := localStep["type"].(string)
	switch t {
	case "powershell":
		return expandStepPowershell(localStep)
	case "cmd_line":
		return expandStepCmdLine(localStep)
	default:
		return nil, fmt.Errorf("Unsupported step type '%s'", t)
	}
}

func expandStepCmdLine(dt map[string]interface{}) (*api.StepCommandLine, error) {
	var file, args, name, code string

	if v, ok := dt["file"]; ok {
		file = v.(string)
	}
	if v, ok := dt["args"]; ok {
		args = v.(string)
	}
	if v, ok := dt["name"]; ok {
		name = v.(string)
	}
	if v, ok := dt["code"]; ok {
		code = v.(string)
	}

	if file != "" {
		return api.NewStepCommandLineExecutable(name, file, args)
	}
	return api.NewStepCommandLineScript(name, code)
}

func expandStepPowershell(dt map[string]interface{}) (*api.StepPowershell, error) {
	var file, args, name, code string

	if v, ok := dt["file"]; ok {
		file = v.(string)
	}
	if v, ok := dt["args"]; ok {
		args = v.(string)
	}
	if v, ok := dt["name"]; ok {
		name = v.(string)
	}
	if v, ok := dt["code"]; ok {
		code = v.(string)
	}

	if file != "" {
		return api.NewStepPowershellScriptFile(name, file, args)
	}
	return api.NewStepPowershellCode(name, code)
}

func buildVcsRootEntry(raw interface{}) *api.VcsRootEntry {
	localVcs := raw.(map[string]interface{})
	rawRules := localVcs["checkout_rules"].([]interface{})
	var toAttachRules string
	if len(rawRules) > 0 {
		stringRules := make([]string, len(rawRules))
		for i, el := range rawRules {
			stringRules[i] = el.(string)
		}
		toAttachRules = strings.Join(stringRules, "\\n")
	}

	return api.NewVcsRootEntryWithRules(&api.VcsRootReference{ID: localVcs["id"].(string)}, toAttachRules)
}

func vcsRootHash(v interface{}) int {
	raw := v.(map[string]interface{})
	return schema.HashString(raw["id"].(string))
}

func stepSetHash(v interface{}) int {
	var buf bytes.Buffer
	m := v.(map[string]interface{})
	buf.WriteString(fmt.Sprintf("%s-", m["type"].(string)))

	if v, ok := m["name"]; ok {
		buf.WriteString(fmt.Sprintf("%s-", v.(string)))
	}

	if v, ok := m["file"]; ok {
		buf.WriteString(fmt.Sprintf("%s-", v.(string)))
	}

	if v, ok := m["args"]; ok {
		buf.WriteString(fmt.Sprintf("%s-", v.(string)))
	}

	return hashcode.String(buf.String())
}
