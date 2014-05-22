package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/zenoss/cli"
	"github.com/zenoss/glog"
	"github.com/zenoss/serviced"
	"github.com/zenoss/serviced/cli/api"
	"github.com/zenoss/serviced/dao"
)

// Initializer for serviced service subcommands
func (c *ServicedCli) initService() {

	defaultMetricsForwarderPort := ":22350"
	if cpConsumerUrl, err := url.Parse(os.Getenv("CONTROLPLANE_CONSUMER_URL")); err == nil {
		hostParts := strings.Split(cpConsumerUrl.Host, ":")
		if len(hostParts) == 2 {
			defaultMetricsForwarderPort = ":" + hostParts[1]
		}
	}

	c.app.Commands = append(c.app.Commands, cli.Command{
		Name:        "service",
		Usage:       "Administers services",
		Description: "",
		Subcommands: []cli.Command{
			{
				Name:         "list",
				Usage:        "Lists all services",
				Description:  "serviced service list [SERVICEID]",
				BashComplete: c.printServicesFirst,
				Action:       c.cmdServiceList,
				Flags: []cli.Flag{
					cli.BoolFlag{"verbose, v", "Show JSON format"},
				},
			}, {
				Name:         "add",
				Usage:        "Adds a new service",
				Description:  "serviced service list NAME POOLID IMAGEID COMMAND",
				BashComplete: c.printServiceAdd,
				Action:       c.cmdServiceAdd,
				Flags: []cli.Flag{
					cli.GenericFlag{"p", &api.PortMap{}, "Expose a port for this service (e.g. -p tcp:3306:mysql)"},
					cli.GenericFlag{"q", &api.PortMap{}, "Map a remote service port (e.g. -q tcp:3306:mysql)"},
				},
			}, {
				Name:         "remove",
				ShortName:    "rm",
				Usage:        "Removes an existing service",
				Description:  "serviced service remove SERVICEID ...",
				BashComplete: c.printServicesAll,
				Action:       c.cmdServiceRemove,
				Flags: []cli.Flag{
					cli.BoolTFlag{"remove-snapshots, R", "Remove snapshots associated with removed service"},
				},
			}, {
				Name:         "edit",
				Usage:        "Edits an existing service in a text editor",
				Description:  "serviced service edit SERVICEID",
				BashComplete: c.printServicesFirst,
				Action:       c.cmdServiceEdit,
				Flags: []cli.Flag{
					cli.StringFlag{"editor, e", os.Getenv("EDITOR"), "Editor used to update the service definition"},
				},
			}, {
				Name:         "assign-ip",
				Usage:        "Assigns an IP address to a service's endpoints requiring an explicit IP address",
				Description:  "serviced service assign-ip SERVICEID [IPADDRESS]",
				BashComplete: c.printServicesFirst,
				Action:       c.cmdServiceAssignIP,
			}, {
				Name:         "start",
				Usage:        "Starts a service",
				Description:  "serviced service start SERVICEID",
				BashComplete: c.printServicesFirst,
				Action:       c.cmdServiceStart,
			}, {
				Name:         "stop",
				Usage:        "Stops a service",
				Description:  "serviced service stop SERVICEID",
				BashComplete: c.printServicesFirst,
				Action:       c.cmdServiceStop,
			}, {
				Name:         "proxy",
				Usage:        "Starts a server proxy for a container",
				Description:  "serviced service proxy SERVICEID INSTANCEID COMMAND",
				BashComplete: c.printServicesFirst,
				Before:       c.cmdServiceProxy,
				Flags: []cli.Flag{
					cli.StringFlag{"forwarder-binary", "/usr/local/serviced/resources/logstash/logstash-forwarder", "path to the logstash-forwarder binary"},
					cli.StringFlag{"forwarder-config", "/etc/logstash-forwarder.conf", "path to the logstash-forwarder config file"},
					cli.IntFlag{"muxport", 22250, "multiplexing port to use"},
					cli.BoolTFlag{"mux", "enable port multiplexing"},
					cli.BoolTFlag{"tls", "enable tls"},
					cli.StringFlag{"keyfile", "", "path to private key file (defaults to compiled in private keys"},
					cli.StringFlag{"certfile", "", "path to public certificate file (defaults to compiled in public cert)"},
					cli.StringFlag{"endpoint", api.GetGateway(defaultRPCPort), "serviced endpoint address"},
					cli.BoolTFlag{"autorestart", "restart process automatically when it finishes"},
					cli.StringFlag{"metric-forwarder-port", defaultMetricsForwarderPort, "the port the container processes send performance data to"},
					cli.BoolTFlag{"logstash", "forward service logs via logstash-forwarder"},
					cli.IntFlag{"v", configInt("LOG_LEVEL", 0), "log level for V logs"},
				},
			}, {
				Name:         "shell",
				Usage:        "Starts a service instance",
				Description:  "serviced service shell SERVICEID COMMAND",
				BashComplete: c.printServicesFirst,
				Before:       c.cmdServiceShell,
				Flags: []cli.Flag{
					cli.StringFlag{"saveas, s", "", "saves the service instance with the given name"},
					cli.BoolFlag{"interactive, i", "runs the service instance as a tty"},
					cli.IntFlag{"v", configInt("LOG_LEVEL", 0), "log level for V logs"},
				},
			}, {
				Name:         "run",
				Usage:        "Runs a service command in a service instance",
				Description:  "serviced service run SERVICEID COMMAND [ARGS]",
				BashComplete: c.printServiceRun,
				Before:       c.cmdServiceRun,
				Flags: []cli.Flag{
					cli.BoolFlag{"interactive, i", "runs the service instance as a tty"},
				},
			}, {
				Name:         "attach",
				Usage:        "Run an arbitrary command in a running service container",
				Description:  "serviced service attach { SERVICEID | SERVICENAME | DOCKERID } [COMMAND]",
				BashComplete: c.printServicesFirst,
				Before:       c.cmdServiceAttach,
			}, {
				Name:         "action",
				Usage:        "Run a predefined action in a running service container",
				Description:  "serviced service action { SERVICEID | SERVICENAME | DOCKERID } ACTION",
				BashComplete: c.printServicesFirst,
				Before:       c.cmdServiceAction,
			}, {
				Name:         "list-snapshots",
				Usage:        "Lists the snapshots for a service",
				Description:  "serviced service list-snapshots SERVICEID",
				BashComplete: c.printServicesFirst,
				Action:       c.cmdServiceListSnapshots,
			}, {
				Name:         "snapshot",
				Usage:        "Takes a snapshot of the service",
				Description:  "serviced service snapshot SERVICEID",
				BashComplete: c.printServicesFirst,
				Action:       c.cmdServiceSnapshot,
			},
		},
	})
}

// Returns a list of all the available service IDs
func (c *ServicedCli) services() (data []string) {
	svcs, err := c.driver.GetServices()
	if err != nil || svcs == nil || len(svcs) == 0 {
		return
	}

	data = make([]string, len(svcs))
	for i, s := range svcs {
		data[i] = s.Id
	}

	return
}

// Returns a list of runnable commands for a particular service
func (c *ServicedCli) serviceRuns(id string) (data []string) {
	svc, err := c.driver.GetService(id)
	if err != nil || svc == nil {
		return
	}

	data = make([]string, len(svc.Runs))
	i := 0
	for r := range svc.Runs {
		data[i] = r
		i++
	}

	return
}

// Returns a list of actionable commands for a particular service
func (c *ServicedCli) serviceActions(id string) (data []string) {
	svc, err := c.driver.GetService(id)
	if err != nil || svc == nil {
		return
	}

	data = make([]string, len(svc.Actions))
	i := 0
	for a := range svc.Actions {
		data[i] = a
		i++
	}

	return
}

// Bash-completion command that prints a list of available services as the
// first argument
func (c *ServicedCli) printServicesFirst(ctx *cli.Context) {
	if len(ctx.Args()) > 0 {
		return
	}
	fmt.Println(strings.Join(c.services(), "\n"))
}

// Bash-completion command that prints a list of available services as all
// arguments
func (c *ServicedCli) printServicesAll(ctx *cli.Context) {
	args := ctx.Args()
	svcs := c.services()

	// If arg is a service don't add to the list
	for _, s := range svcs {
		for _, a := range args {
			if s == a {
				goto next
			}
		}
		fmt.Println(s)
	next:
	}
}

// Bash-completion command that completes the service ID as the first argument
// and runnable commands as the second argument
func (c *ServicedCli) printServiceRun(ctx *cli.Context) {
	var output []string

	args := ctx.Args()
	switch len(args) {
	case 0:
		output = c.services()
	case 1:
		output = c.serviceRuns(args[0])
	}
	fmt.Println(strings.Join(output, "\n"))
}

// Bash-completion command that completes the pool ID as the first argument
// and the docker image ID as the second argument
func (c *ServicedCli) printServiceAdd(ctx *cli.Context) {
	var output []string

	args := ctx.Args()
	switch len(args) {
	case 1:
		output = c.pools()
	case 2:
		// TODO: get a list of the docker images
	}
	fmt.Println(strings.Join(output, "\n"))
}

// serviced service list [--verbose, -v] [SERVICEID]
func (c *ServicedCli) cmdServiceList(ctx *cli.Context) {
	if len(ctx.Args()) > 0 {
		serviceID := ctx.Args()[0]
		if service, err := c.driver.GetService(serviceID); err != nil {
			fmt.Fprintln(os.Stderr, err)
		} else if service == nil {
			fmt.Fprintln(os.Stderr, "service not found")
		} else if jsonService, err := json.MarshalIndent(service, " ", "  "); err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal service definition: %s\n", err)
		} else {
			fmt.Println(string(jsonService))
		}
		return
	}

	services, err := c.driver.GetServices()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	} else if services == nil || len(services) == 0 {
		fmt.Fprintln(os.Stderr, "no services found")
		return
	}

	if ctx.Bool("verbose") {
		if jsonService, err := json.MarshalIndent(services, " ", "  "); err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal service definitions: %s\n", err)
		} else {
			fmt.Println(string(jsonService))
		}
	} else {
		servicemap := api.NewServiceMap(services)
		tableService := newTable(0, 8, 2)
		tableService.PrintRow("NAME", "SERVICEID", "STARTUP", "INST", "IMAGEID", "POOL", "DSTATE", "LAUNCH", "DEPID")

		var printTree func(string)
		printTree = func(root string) {
			services := servicemap.Get(root)
			for i, s := range services {
				tableService.PrintTreeRow(
					!(i+1 < len(services)),
					s.Name,
					s.Id,
					s.Startup,
					s.Instances,
					s.ImageID,
					s.PoolID,
					s.DesiredState,
					s.Launch,
					s.DeploymentID,
				)
				tableService.Indent()
				printTree(s.Id)
				tableService.Dedent()
			}
		}
		printTree("")
		tableService.Flush()
	}
}

// serviced service add [[-p PORT]...] [[-q REMOTE]...] NAME POOLID IMAGEID COMMAND
func (c *ServicedCli) cmdServiceAdd(ctx *cli.Context) {
	args := ctx.Args()
	if len(args) < 4 {
		fmt.Printf("Incorrect Usage.\n\n")
		cli.ShowCommandHelp(ctx, "add")
		return
	}

	cfg := api.ServiceConfig{
		Name:        args[0],
		PoolID:      args[1],
		ImageID:     args[2],
		Command:     args[3],
		LocalPorts:  ctx.Generic("p").(*api.PortMap),
		RemotePorts: ctx.Generic("q").(*api.PortMap),
	}

	if service, err := c.driver.AddService(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if service == nil {
		fmt.Fprintln(os.Stderr, "received nil service definition")
	} else {
		fmt.Println(service.Id)
	}
}

// serviced service remove SERVICEID ...
func (c *ServicedCli) cmdServiceRemove(ctx *cli.Context) {
	args := ctx.Args()
	if len(args) < 1 {
		fmt.Printf("Incorrect Usage.\n\n")
		cli.ShowCommandHelp(ctx, "remove")
		return
	}

	for _, id := range args {
		cfg := api.RemoveServiceConfig{
			ServiceID:       id,
			RemoveSnapshots: ctx.Bool("remove-snapshots"),
		}

		if err := c.driver.RemoveService(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s\n", id, err)
		} else {
			fmt.Println(id)
		}
	}
}

// serviced service edit SERVICEID
func (c *ServicedCli) cmdServiceEdit(ctx *cli.Context) {
	args := ctx.Args()
	if len(args) < 1 {
		fmt.Printf("Incorrect Usage.\n\n")
		cli.ShowCommandHelp(ctx, "edit")
		return
	}

	service, err := c.driver.GetService(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	} else if service == nil {
		fmt.Fprintln(os.Stderr, "service not found")
		return
	}

	jsonService, err := json.MarshalIndent(service, " ", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshalling service: %s\n", err)
		return
	}

	name := fmt.Sprintf("serviced_service_edit_%s", service.Id)
	reader, err := openEditor(jsonService, name, ctx.String("editor"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	if service, err := c.driver.UpdateService(reader); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if service == nil {
		fmt.Fprintln(os.Stderr, "received nil service")
	} else {
		fmt.Println(service.Id)
	}
}

// serviced service assign-ip SERVICEID [IPADDRESS]
func (c *ServicedCli) cmdServiceAssignIP(ctx *cli.Context) {
	args := ctx.Args()
	if len(args) < 1 {
		fmt.Printf("Incorrect Usage.\n\n")
		cli.ShowCommandHelp(ctx, "assign-ip")
		return
	}

	var serviceID, ipAddress string
	serviceID = args[0]
	if len(args) > 1 {
		ipAddress = args[1]
	}

	cfg := api.IPConfig{
		ServiceID: serviceID,
		IPAddress: ipAddress,
	}

	if ipAddress, err := c.driver.AssignIP(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if ipAddress == "" {
		fmt.Fprintln(os.Stderr, "received nil host resource")
	} else {
		fmt.Println(ipAddress)
	}
}

// serviced service start SERVICEID
func (c *ServicedCli) cmdServiceStart(ctx *cli.Context) {
	args := ctx.Args()
	if len(args) < 1 {
		fmt.Printf("Incorrect Usage.\n\n")
		cli.ShowCommandHelp(ctx, "start")
		return
	}

	if host, err := c.driver.StartService(args[0]); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if host == nil {
		fmt.Fprintln(os.Stderr, "received nil host")
	} else {
		fmt.Printf("Service scheduled to start on host: %s\n", host.ID)
	}
}

// serviced service stop SERVICEID
func (c *ServicedCli) cmdServiceStop(ctx *cli.Context) {
	args := ctx.Args()
	if len(args) < 1 {
		fmt.Printf("Incorrect Usage.\n\n")
		cli.ShowCommandHelp(ctx, "stop")
		return
	}

	if err := c.driver.StopService(args[0]); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else {
		fmt.Printf("Service scheduled to stop.\n")
	}
}

// sendLogMessage sends a log message to the host agent
func sendLogMessage(lbClientPort string, serviceLogInfo serviced.ServiceLogInfo) error {
	client, err := serviced.NewLBClient(lbClientPort)
	if err != nil {
		glog.Errorf("Could not create a client to endpoint: %s, %s", lbClientPort, err)
		return err
	}
	defer client.Close()
	return client.SendLogMessage(serviceLogInfo, nil)
}

// serviced service proxy SERVICE_ID INSTANCEID COMMAND
func (c *ServicedCli) cmdServiceProxy(ctx *cli.Context) error {
	if len(ctx.Args()) < 3 {
		fmt.Printf("Incorrect Usage.\n\n")
		return nil
	}

	// Set logging options
	if err := setLogging(ctx); err != nil {
		fmt.Println(err)
	}

	args := ctx.Args()
	options := api.ControllerOptions{
		MuxPort:             ctx.GlobalInt("muxport"),
		Mux:                 ctx.GlobalBool("mux"),
		TLS:                 ctx.GlobalBool("tls"),
		KeyPEMFile:          ctx.GlobalString("keyfile"),
		CertPEMFile:         ctx.GlobalString("certfile"),
		ServicedEndpoint:    ctx.GlobalString("endpoint"),
		Autorestart:         ctx.GlobalBool("autorestart"),
		MetricForwarderPort: ctx.GlobalString("metric-forwarder-port"),
		Logstash:            ctx.GlobalBool("logstash"),
		LogstashBinary:      ctx.GlobalString("forwarder-binary"),
		LogstashConfig:      ctx.GlobalString("forwarder-config"),
		ServiceID:           args[0],
		InstanceID:          args[1],
		Command:             args[2:],
	}

	if err := c.driver.StartProxy(options); err != nil {
		sendLogMessage(options.ServicedEndpoint,
			serviced.ServiceLogInfo{
				ServiceID: options.ServiceID,
				Message:   "container controller terminated with: " + err.Error(),
			})
		fmt.Fprintln(os.Stderr, err)
	}

	return fmt.Errorf("serviced service proxy")
}

// serviced service shell [--saveas SAVEAS]  [--interactive, -i] SERVICEID COMMAND
func (c *ServicedCli) cmdServiceShell(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) < 2 {
		fmt.Printf("Incorrect Usage.\n\n")
		return nil
	}

	// Set logging options
	if err := setLogging(ctx); err != nil {
		fmt.Println(err)
	}

	var (
		serviceID, command string
		argv               []string
		saveAs             string
		isTTY              bool
	)

	serviceID = args[0]
	command = args[1]
	if len(args) > 2 {
		argv = args[2:]
	}
	saveAs = ctx.GlobalString("saveas")
	isTTY = ctx.GlobalBool("interactive")

	config := api.ShellConfig{
		ServiceID: serviceID,
		Command:   command,
		Args:      argv,
		SaveAs:    saveAs,
		IsTTY:     isTTY,
	}

	if err := c.driver.StartShell(config); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	return fmt.Errorf("serviced service shell")
}

// serviced service run SERVICEID [COMMAND [ARGS ...]]
func (c *ServicedCli) cmdServiceRun(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) < 1 {
		fmt.Printf("Incorrect Usage.\n\n")
		return nil
	}

	if len(args) < 2 {
		for _, s := range c.serviceRuns(args[0]) {
			fmt.Println(s)
		}
		return fmt.Errorf("serviced service run")
	}

	var (
		serviceID, command string
		argv               []string
		saveAs             string
		isTTY              bool
	)

	serviceID = args[0]
	command = args[1]
	if len(args) > 2 {
		argv = args[2:]
	}
	saveAs = serviced.GetLabel(serviceID)
	isTTY = ctx.GlobalBool("interactive")

	config := api.ShellConfig{
		ServiceID: serviceID,
		Command:   command,
		Args:      argv,
		SaveAs:    saveAs,
		IsTTY:     isTTY,
	}

	if err := c.driver.RunShell(config); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	return fmt.Errorf("serviced service run")
}

func (c *ServicedCli) searchForRunningService(keyword string) (*dao.RunningService, error) {
	rss, err := c.driver.GetRunningServices()
	if err != nil {
		return nil, err
	}

	var states []*dao.RunningService
	for _, rs := range rss {
		if rs.DockerID == "" {
			continue
		}

		switch keyword {
		case rs.ServiceID, rs.Name, rs.Id, rs.DockerID:
			states = append(states, rs)
		default:
			if keyword == "" {
				states = append(states, rs)
			}
		}
	}

	switch len(states) {
	case 0:
		return nil, fmt.Errorf("no matches found")
	case 1:
		return states[0], nil
	}

	matches := newTable(0, 8, 2)
	matches.PrintRow("NAME", "SERVICEID", "DOCKERID")
	for _, row := range states {
		matches.PrintRow(row.Name, row.ServiceID, row.DockerID)
	}
	matches.Flush()
	return nil, fmt.Errorf("multiple results found; select one from list")
}

// serviced service attach { SERVICEID | SERVICENAME | DOCKERID } [COMMAND ...]
func (c *ServicedCli) cmdServiceAttach(ctx *cli.Context) error {
	// verify args
	args := ctx.Args()
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Incorrect Usage.\n\n")
		cli.ShowCommandHelp(ctx, "attach")
		return nil
	}

	rs, err := c.searchForRunningService(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}

	var command string
	if len(args) > 1 {
		command = args[1]
	}

	var argv []string
	if len(args) > 2 {
		argv = args[2:]
	}

	cfg := api.AttachConfig{
		Running: rs,
		Command: command,
		Args:    argv,
	}

	if err := c.driver.Attach(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	return fmt.Errorf("serviced service attach")
}

// serviced service action { SERVICEID | SERVICENAME | DOCKERID } ACTION
func (c *ServicedCli) cmdServiceAction(ctx *cli.Context) error {
	// verify args
	args := ctx.Args()
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Incorrect Usage.\n\n")
		cli.ShowCommandHelp(ctx, "action")
		return nil
	}

	rs, err := c.searchForRunningService(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}

	switch len(args) {
	case 1:
		actions := c.serviceActions(rs.ServiceID)
		if len(actions) > 0 {
			fmt.Println(strings.Join(actions, "\n"))
		} else {
			fmt.Fprintln(os.Stderr, "no actions found")
		}
	default:
		cfg := api.AttachConfig{
			Running: rs,
			Command: args[1],
		}
		if len(args) > 2 {
			cfg.Args = args[2:]
		}

		if err := c.driver.Action(cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}

	return fmt.Errorf("serviced service attach")
}

// serviced service list-snapshot SERVICEID
func (c *ServicedCli) cmdServiceListSnapshots(ctx *cli.Context) {
	if len(ctx.Args()) < 1 {
		fmt.Printf("Incorrect Usage.\n\n")
		cli.ShowCommandHelp(ctx, "list-snapshots")
		return
	}

	if snapshots, err := c.driver.GetSnapshotsByServiceID(ctx.Args().First()); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if snapshots == nil || len(snapshots) == 0 {
		fmt.Fprintln(os.Stderr, "no snapshots found")
	} else {
		for _, s := range snapshots {
			fmt.Println(s)
		}
	}
}

// serviced service snapshot SERVICEID
func (c *ServicedCli) cmdServiceSnapshot(ctx *cli.Context) {
	if len(ctx.Args()) < 1 {
		fmt.Printf("Incorrect Usage.\n\n")
		cli.ShowCommandHelp(ctx, "snapshot")
		return
	}

	if snapshot, err := c.driver.AddSnapshot(ctx.Args().First()); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if snapshot == "" {
		fmt.Fprintln(os.Stderr, "received nil snapshot")
	} else {
		fmt.Println(snapshot)
	}
}
