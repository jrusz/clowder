package serviceaccount

import (
	"fmt"
	"strings"

	crd "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	"github.com/RedHatInsights/clowder/controllers/cloud.redhat.com/providers"
	"github.com/RedHatInsights/clowder/controllers/cloud.redhat.com/providers/database"
	"github.com/RedHatInsights/clowder/controllers/cloud.redhat.com/providers/deployment"
	"github.com/RedHatInsights/clowder/controllers/cloud.redhat.com/providers/featureflags"
	"github.com/RedHatInsights/clowder/controllers/cloud.redhat.com/providers/inmemorydb"
	"github.com/RedHatInsights/clowder/controllers/cloud.redhat.com/providers/objectstore"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"

	rc "github.com/RedHatInsights/rhc-osdk-utils/resource_cache"
	"github.com/RedHatInsights/rhc-osdk-utils/utils"
)

type serviceaccountProvider struct {
	providers.Provider
}

func NewServiceAccountProvider(p *providers.Provider) (providers.ClowderProvider, error) {
	return &serviceaccountProvider{Provider: *p}, nil
}

func (sa *serviceaccountProvider) EnvProvide() error {
	if err := createServiceAccountForClowdObj(sa.Cache, CoreEnvServiceAccount, sa.Env); err != nil {
		return err
	}

	resourceIdentsToUpdate := []rc.ResourceIdent{
		featureflags.LocalFFDBDeployment,
		objectstore.MinioDeployment,
		database.SharedDBDeployment,
	}

	for _, resourceIdent := range resourceIdentsToUpdate {
		if obj, ok := resourceIdent.(rc.ResourceIdentSingle); ok {
			dd := &apps.Deployment{}
			if err := sa.Cache.Get(obj, dd); err != nil {
				if strings.Contains(err.Error(), "not found") {
					continue
				}
			}
			dd.Spec.Template.Spec.ServiceAccountName = sa.Env.GetClowdSAName()
			if err := sa.Cache.Update(obj, dd); err != nil {
				return err
			}
		}
	}
	return nil
}

func (sa *serviceaccountProvider) Provide(app *crd.ClowdApp) error {

	if err := createIQEServiceAccounts(&sa.Provider, app); err != nil {
		return err
	}

	if err := createServiceAccountForClowdObj(sa.Cache, CoreAppServiceAccount, app); err != nil {
		return err
	}

	resourceIdentsToUpdate := []rc.ResourceIdent{
		database.LocalDBDeployment,
		inmemorydb.RedisDeployment,
	}

	for _, resourceIdent := range resourceIdentsToUpdate {
		if obj, ok := resourceIdent.(rc.ResourceIdentSingle); ok {
			dd := &apps.Deployment{}
			if err := sa.Cache.Get(obj, dd); err != nil {
				if strings.Contains(err.Error(), "not found") {
					continue
				}
			}
			dd.Spec.Template.Spec.ServiceAccountName = app.GetClowdSAName()
			if err := sa.Cache.Update(obj, dd); err != nil {
				return err
			}
		}
	}

	for _, dep := range app.Spec.Deployments {
		d := &apps.Deployment{}
		nn := app.GetDeploymentNamespacedName(&dep)

		if err := sa.Cache.Get(deployment.CoreDeployment, d, nn); err != nil {
			return err
		}

		labeler := utils.GetCustomLabeler(nil, nn, app)

		if err := CreateServiceAccount(sa.Cache, CoreDeploymentServiceAccount, nn, labeler); err != nil {
			return err
		}

		d.Spec.Template.Spec.ServiceAccountName = nn.Name
		if err := sa.Cache.Update(deployment.CoreDeployment, d); err != nil {
			return err
		}

		if err := CreateRoleBinding(sa.Cache, CoreDeploymentRoleBinding, nn, labeler, dep.K8sAccessLevel); err != nil {
			return err
		}

	}

	return nil
}

func createIQEServiceAccounts(p *providers.Provider, app *crd.ClowdApp) error {

	accessLevel := p.Env.Spec.Providers.Testing.K8SAccessLevel

	nn := types.NamespacedName{
		Name:      fmt.Sprintf("iqe-%s", p.Env.Name),
		Namespace: app.Namespace,
	}

	labeler := utils.GetCustomLabeler(nil, nn, p.Env)
	if err := CreateServiceAccount(p.Cache, IQEServiceAccount, nn, labeler); err != nil {
		return err
	}

	switch accessLevel {
	// Use edit level service account to create and delete resources
	// one per app when the app is created
	case "edit", "view":
		if err := CreateRoleBinding(p.Cache, IQERoleBinding, nn, labeler, accessLevel); err != nil {
			return err
		}

	default:
	}

	return nil
}
