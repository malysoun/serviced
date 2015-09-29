package dfs

import (
	"sync"
	"time"

	"github.com/control-center/serviced/dao"
	"github.com/control-center/serviced/datastore"
	"github.com/control-center/serviced/domain/host"
	"github.com/control-center/serviced/domain/pool"
	"github.com/control-center/serviced/domain/service"
	"github.com/control-center/serviced/domain/servicetemplate"
	"github.com/control-center/serviced/facade"
)

type Database interface {
	UpdateService(svc *service.Service) error
	GetServiceTemplates() ([]servicetemplate.ServiceTemplate, error)
	GetResourcePools() ([]pool.ResourcePool, error)
	GetHosts() ([]host.Host, error)
	GetTenantIDs() ([]string, error)
	GetTenantLock(tenantID string) sync.Locker
	GetServices(tenantID string) ([]service.Service, error)
	ScheduleService(serviceID string, desiredState service.DesiredState) error
	WaitServices(desiredState service.DesiredState, serviceIDs []string) error
	RestoreServiceTemplates(templates []servicetemplate.ServiceTemplate) error
	RestoreResourcePools(pools []pool.ResourcePool) error
	RestoreHosts(hosts []host.Host) error
	RestoreServices(tenantID string, svcs []service.Service) error
}

type database struct {
	timeout time.Duration
	facade  *facade.Facade
}

func (db *database) UpdateService(svc *service.Service) error {
	ctx := datastore.Get()
	return db.facade.UpdateService(ctx, svc)
}

func (db *database) GetServiceTemplates() ([]servicetemplate.ServiceTemplate, error) {
	ctx := datastore.Get()
	tmap, err := db.facade.GetServiceTemplates(ctx)
	if err != nil {
		return nil, err
	}
	templates := make([]servicetemplate.ServiceTemplate, 0)
	for _, t := range tmap {
		templates = append(templates, t)
	}
	return templates, nil
}

func (db *database) GetResourcePools() ([]pool.ResourcePool, error) {
	ctx := datastore.Get()
	return db.facade.GetResourcePools(ctx)
}

func (db *database) GetHosts() ([]host.Host, error) {
	ctx := datastore.Get()
	return db.facade.GetHosts(ctx)
}

func (db *database) GetTenantIDs() ([]string, error) {
	ctx := datastore.Get()
	return db.facade.GetTenantIDs(ctx)
}

func (db *database) GetTenantLock(tenantID string) sync.Locker {
	ctx := datastore.Get()
	return db.facade.GetTenantLock(tenantID)
}

func (db *database) GetServices(tenantID string) ([]service.Service, error) {
	ctx := datastore.Get()
	return db.facade.GetServices(ctx, dao.ServiceRequest{TenantID: tenantID})
}

func (db *database) ScheduleService(serviceID string, dstate service.DesiredState) error {
	ctx := datastore.Get()
	_, err := db.facade.ScheduleService(ctx, serviceID, false, dstate)
	return err
}

func (db *database) WaitServices(serviceIDs []string, dstate service.DesiredState) error {
	ctx := datastore.Get()
	return db.facade.WaitService(ctx, dstate, db.timeout, serviceIDs...)
}

func (db *database) RestoreServiceTemplates(templates []servicetemplate.ServiceTemplate) error {
	ctx := datastore.Get()
	return db.facade.RestoreServiceTemplates(ctx, templates)
}

func (db *database) RestoreResourcePools(pools []pool.ResourcePool) error {
	ctx := datastore.Get()
	return db.facade.RestoreResourcePools(ctx, pools)
}

func (db *database) RestoreHosts(hosts []host.Host) error {
	ctx := datastore.Get()
	return db.facade.RestoreHosts(ctx, hosts)
}

func (db *databade) RestoreServices(tenantID string, svcs []service.Service) error {
	ctx := datastore.Get()
	return db.facade.RestoreServices(tenantID, svcs)
}
