package main

import (
	"fmt"
	"strings"
)

const version1 = "2015-07-25"
const version2 = "2015-12-19"
const version3 = "2016-07-29"

var (
	versionList = []string{version1, version2, version3}
)

func GenerateAnswers(data []map[string]interface{}) (Versions, error) {
	versions := make(map[string]Answers)

	for _, v := range versionList {
		// 1. generate interim data
		interim := &Interim{
			UUIDToService:                   make(map[string]map[string]interface{}),
			UUIDToContainer:                 make(map[string]map[string]interface{}),
			UUIDToStack:                     make(map[string]map[string]interface{}),
			UUIDToHost:                      make(map[string]map[string]interface{}),
			StackUUIDToServicesUUID:         make(map[string][]string),
			ServiceUUIDNameToContainersUUID: make(map[string][]string),
			ContainerUUIDToContainerLink:    make(map[string]map[string]interface{}),
			ServiceUUIDToServiceLink:        make(map[string]map[string]interface{}),
			Networks:                        []interface{}{},
			Default:                         make(map[string]interface{}),
		}

		for _, o := range data {
			no := make(map[string]interface{}, len(o))
			for k, v := range o {
				no[k] = v
			}
			processMetadataObject(no, interim)
		}
		// 2. Generate versions from temp data
		if err := generateVersions(interim, v, versions); err != nil {
			return nil, err
		}
	}

	//tag the latest
	versions["latest"] = versions[version3]
	return versions, nil
}

func generateVersions(interim *Interim, version string, versions Versions) error {
	versionedData, err := applyVersionToData(*interim, version)
	if err != nil {
		return err
	}
	addToVersions(versions, version, versionedData)
	return nil
}

func addToVersions(versions Versions, version string, versionedData *Interim) {
	answers := make(map[string]interface{})
	defaultAnswers := addDefaultToAnswers(answers, versionedData)
	addClientToAnswers(answers, defaultAnswers, versionedData)
	versions[version] = answers
}

func addClientToAnswers(answers Answers, defaultAnswers map[string]interface{}, versionedData *Interim) {
	for _, c := range versionedData.UUIDToContainer {
		if c["primary_ip"] == nil {
			continue
		}
		clientAnswers := make(map[string]interface{})
		self := make(map[string]interface{})
		self["container"] = c
		if c["stack_uuid"] != nil {
			self["stack"] = versionedData.UUIDToStack[c["stack_uuid"].(string)]
			service := versionedData.UUIDToService[getServiceUUID(c["service_uuid"].(string), c["service_name"].(string))]
			selfService := make(map[string]interface{})
			// to exclude token from service
			for k, v := range service {
				selfService[k] = v
			}
			service["token"] = nil
			self["service"] = selfService
		}
		if c["host_uuid"] != nil {
			self["host"] = versionedData.UUIDToHost[c["host_uuid"].(string)]
		}
		clientAnswers["self"] = self
		mergeDefaults(clientAnswers, defaultAnswers)
		answers[c["primary_ip"].(string)] = clientAnswers
	}
}

func mergeDefaults(clientAnswers map[string]interface{}, defaultAnswers map[string]interface{}) {
	for k, v := range defaultAnswers {
		_, exists := clientAnswers[k]
		if !exists {
			clientAnswers[k] = v
		}
	}
}

func addDefaultToAnswers(answers Answers, versionedData *Interim) map[string]interface{} {
	defaultAnswers := make(map[string]interface{})
	var containers []interface{}
	for _, c := range versionedData.UUIDToContainer {
		if _, ok := defaultAnswers["containers"]; ok {
			containers = defaultAnswers["containers"].([]interface{})
		}
		containers = append(containers, c)
	}
	defaultAnswers["containers"] = containers

	var stacks []interface{}
	for _, s := range versionedData.UUIDToStack {
		if _, ok := defaultAnswers["stacks"]; ok {
			stacks = defaultAnswers["stacks"].([]interface{})
		}
		stacks = append(stacks, s)
	}
	defaultAnswers["stacks"] = stacks

	var services []interface{}
	for _, s := range versionedData.UUIDToService {
		if _, ok := defaultAnswers["services"]; ok {
			services = defaultAnswers["services"].([]interface{})
		}
		services = append(services, s)
	}
	defaultAnswers["services"] = services

	var hosts []interface{}
	for _, h := range versionedData.UUIDToHost {
		if _, ok := defaultAnswers["hosts"]; ok {
			hosts = defaultAnswers["hosts"].([]interface{})
		}
		hosts = append(hosts, h)
	}
	defaultAnswers["hosts"] = hosts
	defaultAnswers["networks"] = versionedData.Networks

	if val, ok := versionedData.Default["version"]; ok {
		defaultAnswers["version"] = val
	}

	if selfVal, ok := versionedData.Default["self"]; ok {
		self := selfVal.(map[string]interface{})
		if hostVal, ok := self["host"]; ok {
			host := hostVal.(map[string]interface{})
			self["host"] = versionedData.UUIDToHost[host["uuid"].(string)]
		}
		defaultAnswers["self"] = self
	}

	rancherInstallInfo := make(map[string]interface{})
	rancherInstallInfo["uuid"] = INSTALL_UUID
	defaultAnswers["install"] = rancherInstallInfo

	answers[DEFAULT_KEY] = defaultAnswers
	return defaultAnswers
}

func applyVersionToData(modified Interim, version string) (*Interim, error) {
	// 1. Process containers
	for _, c := range modified.UUIDToContainer {
		switch version {
		case version3:
			if c["name"] != nil {
				c["name"] = strings.ToLower(c["name"].(string))
			}
			if c["service_name"] != nil {
				c["service_name"] = strings.ToLower(c["service_name"].(string))
			}
			if c["stack_name"] != nil {
				c["stack_name"] = strings.ToLower(c["stack_name"].(string))
			}
			if _, ok := c["ports"]; ok {
				// set port ip to 0.0.0.0 if not specified
				originalPorts := c["ports"].([]interface{})
				var newPorts []interface{}
				for _, p := range originalPorts {
					port := p.(string)
					splitted := strings.Split(port, ":")
					if len(splitted) == 3 {
						newPorts = append(newPorts, port)
					} else {
						port = fmt.Sprintf("0.0.0.0:%s", port)
						newPorts = append(newPorts, port)
					}
				}
				c["ports"] = newPorts
			}

		default:
			if _, ok := c["ports"]; ok {
				originalPorts := c["ports"].([]interface{})
				// set port ip to host's ip if not specified
				if len(originalPorts) > 0 {
					var newPorts []interface{}
					for _, p := range originalPorts {
						port := p.(string)
						splitted := strings.Split(port, ":")
						if len(splitted) == 3 && splitted[0] != "0.0.0.0" {
							newPorts = append(newPorts, port)
						} else {
							if len(splitted) == 3 {
								port = fmt.Sprintf("%s%s", c["host_ip"], strings.TrimPrefix(port, "0.0.0.0"))
							} else {
								port = fmt.Sprintf("%s:%s", c["host_ip"], port)
							}

							newPorts = append(newPorts, port)
						}
					}
					c["ports"] = newPorts
				}
			}
		}
		c["links"] = modified.ContainerUUIDToContainerLink[c["uuid"].(string)]
		//delete helper field (needed for the port)
		delete(c, "host_ip")
	}

	// 2. Process services
	for _, s := range modified.UUIDToService {
		sUUID := getServiceUUID(s["uuid"].(string), s["name"].(string))

		stackUUID := s["stack_uuid"].(string)
		// add itself to the stack list
		var svcUUIDs []string
		if _, ok := modified.StackUUIDToServicesUUID[stackUUID]; ok {
			svcUUIDs = modified.StackUUIDToServicesUUID[stackUUID]
		}
		svcUUIDs = append(svcUUIDs, getServiceUUID(s["uuid"].(string), s["name"].(string)))
		modified.StackUUIDToServicesUUID[stackUUID] = svcUUIDs
		var cs []interface{}
		var cNames []interface{}
		cUUIDs := modified.ServiceUUIDNameToContainersUUID[sUUID]
		if cUUIDs != nil {
			for _, cUUID := range cUUIDs {
				if c, ok := modified.UUIDToContainer[cUUID]; ok {
					cs = append(cs, c)
					cNames = append(cNames, c["name"].(string))
				}
			}
		}
		// add service links
		s["links"] = modified.ServiceUUIDToServiceLink[s["uuid"].(string)]
		switch version {
		case version1:
			s["containers"] = cNames
		case version2:
			s["containers"] = cs
		case version3:
			s["containers"] = cs
			s["name"] = strings.ToLower(s["name"].(string))
			s["stack_name"] = strings.ToLower(s["stack_name"].(string))
			s["primary_service_name"] = strings.ToLower(s["primary_service_name"].(string))
			if s["sidekicks"] != nil {
				sidekicks := s["sidekicks"].([]interface{})
				var lowercased []interface{}
				for _, sidekick := range sidekicks {
					lowercased = append(lowercased, strings.ToLower(sidekick.(string)))
				}
				s["sidekicks"] = lowercased
			}
			if s["links"] != nil {
				links := s["links"].(map[string]interface{})
				lowercased := make(map[string]interface{})
				for key, value := range links {
					lowercased[strings.ToLower(key)] = value
				}
				s["links"] = lowercased
			}
		}

		// populate stack/service info on container
		if cUUIDs != nil {
			for _, cUUID := range cUUIDs {
				if _, ok := modified.UUIDToContainer[cUUID]; ok {
					modified.UUIDToContainer[cUUID]["service_name"] = s["name"]
					modified.UUIDToContainer[cUUID]["service_uuid"] = s["uuid"]
					modified.UUIDToContainer[cUUID]["stack_name"] = s["stack_name"]
					modified.UUIDToContainer[cUUID]["stack_uuid"] = stackUUID
				}
			}
		}
	}

	// 3. Process stacks
	for _, s := range modified.UUIDToStack {
		var svcs []interface{}
		var svcsNames []interface{}
		svcsUUIDs := modified.StackUUIDToServicesUUID[s["uuid"].(string)]
		if svcsUUIDs != nil {
			for _, svcUUID := range svcsUUIDs {
				if svc, ok := modified.UUIDToService[svcUUID]; ok {
					svcs = append(svcs, svc)
					svcsNames = append(svcsNames, svc["name"].(string))
				}
			}
		}
		switch version {
		case version1:
			s["services"] = svcsNames
		case version2:
			s["services"] = svcs
		case version3:
			s["services"] = svcs
			s["name"] = strings.ToLower(s["name"].(string))
		}
	}

	// 4. process hosts
	for _, h := range modified.UUIDToHost {
		switch version {
		case version3:
			delete(h, "hostId")
		}
	}

	return &modified, nil
}

func processMetadataObject(o map[string]interface{}, interim *Interim) {
	if val, ok := o["metadata_kind"]; ok {
		switch val.(string) {
		case "container":
			addContainer(o, interim)
		case "stack":
			addStack(o, interim)
		case "network":
			addNetwork(o, interim)
		case "host":
			addHost(o, interim)
		case "defaultData":
			addDefault(o, interim)
		case "serviceContainerLink":
			addServiceContainerLink(o, interim)
		case "containerLink":
			addContainerLink(o, interim)
		case "serviceLink":
			addServiceLink(o, interim)
		case "service":
			addService(o, interim)
		}
	}
}

func addContainer(container map[string]interface{}, interim *Interim) {
	interim.UUIDToContainer[container["uuid"].(string)] = container
}

func addService(service map[string]interface{}, interim *Interim) {
	interim.UUIDToService[getServiceUUID(service["uuid"].(string), service["name"].(string))] = service
}

func addServiceContainerLink(link map[string]interface{}, interim *Interim) {
	UUID := getServiceUUID(link["service_uuid"].(string), link["service_name"].(string))
	var containerUUIDList []string
	if _, ok := interim.ServiceUUIDNameToContainersUUID[UUID]; ok {
		containerUUIDList = interim.ServiceUUIDNameToContainersUUID[UUID]
	}
	containerUUIDList = append(containerUUIDList, link["container_uuid"].(string))
	interim.ServiceUUIDNameToContainersUUID[UUID] = containerUUIDList
}

func addServiceLink(link map[string]interface{}, interim *Interim) {
	serviceUUID := link["service_uuid"].(string)
	links := make(map[string]interface{})
	if _, ok := interim.ServiceUUIDToServiceLink[serviceUUID]; ok {
		links = interim.ServiceUUIDToServiceLink[serviceUUID]
	}
	linkKey := link["key"].(string)
	links[linkKey] = link["value"].(string)

	interim.ServiceUUIDToServiceLink[serviceUUID] = links
}

func addContainerLink(link map[string]interface{}, interim *Interim) {
	containerUUID := link["container_uuid"].(string)
	links := make(map[string]interface{})
	if _, ok := interim.ContainerUUIDToContainerLink[containerUUID]; ok {
		links = interim.ContainerUUIDToContainerLink[containerUUID]
	}
	linkKey := link["key"].(string)
	links[linkKey] = link["value"].(string)

	interim.ContainerUUIDToContainerLink[containerUUID] = links
}

func getServiceUUID(UUID string, name string) string {
	return strings.ToLower(fmt.Sprintf("%s_%s", UUID, name))
}

func addStack(stack map[string]interface{}, interim *Interim) {
	interim.UUIDToStack[stack["uuid"].(string)] = stack
}

func addNetwork(network map[string]interface{}, interim *Interim) {
	interim.Networks = append(interim.Networks, network)
}

func addHost(host map[string]interface{}, interim *Interim) {
	interim.UUIDToHost[host["uuid"].(string)] = host
}

func addDefault(def map[string]interface{}, interim *Interim) {
	interim.Default = def
}
