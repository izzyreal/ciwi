package uidsl

import (
	"fmt"
	"strings"
)

const RoutesKind = "Routes"

type RouteDocument struct {
	APIVersion string      `yaml:"apiVersion" json:"apiVersion"`
	Kind       string      `yaml:"kind" json:"kind"`
	Routes     []RouteSpec `yaml:"routes" json:"routes"`
}

type RouteSpec struct {
	Name        string   `yaml:"name" json:"name"`
	Pattern     string   `yaml:"pattern" json:"pattern"`
	Screen      string   `yaml:"screen" json:"screen"`
	BindingRoot string   `yaml:"bindingRoot" json:"bindingRoot"`
	Platforms   []string `yaml:"platforms" json:"platforms"`
}

type RouteMatch struct {
	Route  RouteSpec
	Params map[string]string
}

func ParseRoutes(payload []byte) (*RouteDocument, error) {
	var document RouteDocument
	if err := decodeStrict(payload, &document); err != nil {
		return nil, err
	}
	if err := document.Validate(); err != nil {
		return nil, err
	}
	return &document, nil
}

func (d *RouteDocument) Validate() error {
	if d == nil {
		return fmt.Errorf("route document is nil")
	}
	if d.APIVersion != APIVersion {
		return fmt.Errorf("unsupported apiVersion %q", d.APIVersion)
	}
	if d.Kind != RoutesKind {
		return fmt.Errorf("unsupported route document kind %q", d.Kind)
	}
	if len(d.Routes) == 0 {
		return fmt.Errorf("route document contains no routes")
	}
	names := map[string]bool{}
	patterns := map[string]bool{}
	for index, route := range d.Routes {
		if !identifierPattern.MatchString(route.Name) {
			return fmt.Errorf("route %d has invalid name %q", index, route.Name)
		}
		if names[route.Name] {
			return fmt.Errorf("duplicate route name %q", route.Name)
		}
		names[route.Name] = true
		if route.Pattern == "" || route.Pattern[0] != '/' || (route.Pattern != "/" && strings.HasSuffix(route.Pattern, "/")) {
			return fmt.Errorf("route %q has invalid pattern %q", route.Name, route.Pattern)
		}
		if patterns[route.Pattern] {
			return fmt.Errorf("duplicate route pattern %q", route.Pattern)
		}
		patterns[route.Pattern] = true
		if !identifierPattern.MatchString(route.Screen) || !identifierPattern.MatchString(route.BindingRoot) {
			return fmt.Errorf("route %q has invalid screen or binding root", route.Name)
		}
		params := map[string]bool{}
		for _, segment := range routeSegments(route.Pattern) {
			if !strings.HasPrefix(segment, "{") && !strings.HasSuffix(segment, "}") {
				continue
			}
			if len(segment) < 3 || segment[0] != '{' || segment[len(segment)-1] != '}' {
				return fmt.Errorf("route %q has malformed parameter %q", route.Name, segment)
			}
			name := segment[1 : len(segment)-1]
			if !identifierPattern.MatchString(name) || params[name] {
				return fmt.Errorf("route %q has invalid or duplicate parameter %q", route.Name, name)
			}
			params[name] = true
		}
		if len(route.Platforms) == 0 {
			return fmt.Errorf("route %q has no platforms", route.Name)
		}
		seenPlatforms := map[string]bool{}
		for _, platform := range route.Platforms {
			if platform != "web" && platform != "gio" {
				return fmt.Errorf("route %q has unsupported platform %q", route.Name, platform)
			}
			if seenPlatforms[platform] {
				return fmt.Errorf("route %q repeats platform %q", route.Name, platform)
			}
			seenPlatforms[platform] = true
		}
	}
	return nil
}

func (d *RouteDocument) Match(path, platform string) (RouteMatch, bool) {
	if d == nil {
		return RouteMatch{}, false
	}
	path = strings.TrimSpace(path)
	if path != "/" {
		path = strings.TrimSuffix(path, "/")
	}
	actual := routeSegments(path)
	for _, route := range d.Routes {
		if !routeSupportsPlatform(route, platform) {
			continue
		}
		expected := routeSegments(route.Pattern)
		if len(actual) != len(expected) {
			continue
		}
		params := map[string]string{}
		matched := true
		for index := range expected {
			segment := expected[index]
			if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
				if actual[index] == "" {
					matched = false
					break
				}
				params[segment[1:len(segment)-1]] = actual[index]
				continue
			}
			if segment != actual[index] {
				matched = false
				break
			}
		}
		if matched {
			return RouteMatch{Route: route, Params: params}, true
		}
	}
	return RouteMatch{}, false
}

func routeSupportsPlatform(route RouteSpec, platform string) bool {
	for _, candidate := range route.Platforms {
		if candidate == platform {
			return true
		}
	}
	return false
}

func routeSegments(path string) []string {
	if path == "/" || path == "" {
		return nil
	}
	return strings.Split(strings.Trim(path, "/"), "/")
}
