package teamcity

import (
	"log"

	api "github.com/cvbarros/go-teamcity-sdk/pkg/teamcity"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceProject() *schema.Resource {
	return &schema.Resource{
		Create: resourceProjectCreate,
		Read:   resourceProjectRead,
		Update: resourceProjectUpdate,
		Delete: resourceProjectDelete,
		Importer: &schema.ResourceImporter{
			State: resourceProjectImport,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
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

func resourceProjectCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*api.Client)
	var name string

	if v, ok := d.GetOk("name"); ok {
		name = v.(string)
	}

	newProj, err := api.NewProject(name, "", "")
	if err != nil {
		return err
	}

	created, err := client.Projects.Create(newProj)
	if err != nil {
		return err
	}

	d.SetId(created.ID)
	d.MarkNewResource()

	return resourceProjectUpdate(d, client)
}

func resourceProjectRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*api.Client)

	dt, err := getProject(client, d.Id())
	if err != nil {
		return err
	}

	if err := d.Set("name", dt.Name); err != nil {
		return err
	}
	if err := d.Set("description", dt.Description); err != nil {
		return err
	}

	flattenParameterCollection(d, dt.Parameters)

	log.Printf("[DEBUG] Project: %v", dt)
	return nil
}

func resourceProjectUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*api.Client)
	dt, err := client.Projects.GetByID(d.Id())
	if err != nil {
		return err
	}

	if v, ok := d.GetOk("description"); ok {
		dt.Description = v.(string)
	}
	dt.Parameters, err = expandParameterCollection(d)
	if err != nil {
		return err
	}

	_, err = client.Projects.Update(dt)
	return err
}

func resourceProjectDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*api.Client)
	return client.Projects.Delete(d.Id())
}

func resourceProjectImport(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	if err := resourceProjectRead(d, meta); err != nil {
		return nil, err
	}
	return []*schema.ResourceData{d}, nil
}

func getProject(c *api.Client, id string) (*api.Project, error) {
	dt, err := c.Projects.GetByID(id)
	if err != nil {
		return nil, err
	}

	return dt, nil
}
