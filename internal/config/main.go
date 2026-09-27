package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kraken-hub/kraken/pkg/template"
	"github.com/kraken-hub/kraken/pkg/yaml"
)

func LoadYamlFile(file string, env func(string) string) (string, error) {
	data, err := template.RenderTemplateFile(file, env)
	if err != nil {
		return "", err
	}

	node, err := yaml.Load([]byte(data))
	if err != nil {
		return "", err
	}

	err = PrepareYaml(node, env("APP_ENV"))
	if err != nil {
		return "", err
	}

	dump, err := yaml.Dump(node)
	if err != nil {
		return "", err
	}

	return string(dump), nil
}

func PrepareYaml(node yaml.Node, env string) error {
	ctx := yaml.NewCtx()
	ctx.Index(node)

	yaml.RemoveAllComments(node)
	prepareEnvNode(ctx, node, env)

	rootNode, ok := yaml.Resolve(ctx, node).(*yaml.MappingNode)
	if !ok {
		addAnchors(node)
		return nil
	}

	_, servicesVal := yaml.FindValue(ctx, rootNode, "services")
	services, _ := yaml.Resolve(ctx, servicesVal).(*yaml.MappingNode)

	if services != nil {
		for i := 0; i < len(services.Values); i++ {
			svcPair := services.Values[i]
			svcMap, ok := yaml.Resolve(ctx, svcPair.Value).(*yaml.MappingNode)
			if !ok {
				continue
			}

			if _, envVal := yaml.FindValue(ctx, svcMap, "env"); envVal != nil {
				if _, ok := yaml.Resolve(ctx, envVal).(*yaml.SequenceNode); ok {
					idx, _ := yaml.FindPair(ctx, svcMap, "env")
					yaml.ReplacePairValue(svcMap, idx, convertSequenceToMapping(ctx, envVal, "="))
				}
			}

			if idx, extends := yaml.FindValue(ctx, svcMap, "extends"); extends != nil {
				yaml.RemovePair(svcMap, idx)

				if resolved, ok := yaml.Resolve(ctx, extends).(*yaml.MappingNode); ok {
					for _, mv := range resolved.Values {
						if yaml.AnchorName(mv.Value) != "" {
							replaceAnchorUsageNode(ctx, rootNode, mv.Value)
						}
					}
				}

				_, fileVal := yaml.FindValue(ctx, extends, "file")
				filePath, err := filepath.Abs(yaml.ScalarText(fileVal))
				if err != nil {
					return err
				}

				data, err := LoadYamlFile(filePath, os.Getenv)
				if err != nil {
					return err
				}

				exNode, err := yaml.Load([]byte(data))
				if err != nil {
					return err
				}

				exCtx := yaml.NewCtx()
				exCtx.Index(exNode)

				_, serviceVal := yaml.FindValue(ctx, extends, "service")
				_, exServicesVal := yaml.FindValue(exCtx, exNode, "services")
				exServices, _ := yaml.Resolve(exCtx, exServicesVal).(*yaml.MappingNode)

				if exServices != nil {
					replaceName := yaml.ScalarText(serviceVal)
					_, fn := yaml.FindValue(ctx, extends, "fn")
					fn = yaml.Resolve(ctx, fn)

					if err := ExtendYamlService(services, exServices, fn, i, replaceName, nil, nil, false); err != nil {
						return err
					}
				}
			}

			svcMap, ok = yaml.Resolve(ctx, svcPair.Value).(*yaml.MappingNode)
			if !ok {
				continue
			}

			if idx, extend := yaml.FindValue(ctx, svcMap, "extend"); extend != nil {
				yaml.RemovePair(svcMap, idx)

				if resolved, ok := yaml.Resolve(ctx, extend).(*yaml.MappingNode); ok {
					for _, mv := range resolved.Values {
						if yaml.AnchorName(mv.Value) != "" {
							replaceAnchorUsageNode(ctx, rootNode, mv.Value)
						}
					}
				}

				if yaml.IsScalar(extend) {
					extendVal := yaml.ScalarText(extend)
					var includeKeys, excludeKeys []string
					if idx := strings.Index(extendVal, "&"); idx >= 0 {
						includeKeys = strings.Split(extendVal[idx+1:], ",")
						extendVal = extendVal[:idx]
					} else if idx := strings.Index(extendVal, "!"); idx >= 0 {
						excludeKeys = strings.Split(extendVal[idx+1:], ",")
						extendVal = extendVal[:idx]
					}
					if err := ExtendYamlService(services, services, nil, i, extendVal, includeKeys, excludeKeys, true); err != nil {
						return err
					}
				} else {
					_, serviceVal := yaml.FindValue(ctx, extend, "service")
					replaceName := yaml.ScalarText(serviceVal)
					_, fn := yaml.FindValue(ctx, extend, "fn")
					fn = yaml.Resolve(ctx, fn)

					var includeKeys, excludeKeys []string
					_, typeVal := yaml.FindValue(ctx, extend, "type")
					_, keysVal := yaml.FindValue(ctx, extend, "keys")
					if typeVal != nil && keysVal != nil {
						keys := sequenceNodeToStrings(ctx, keysVal)
						switch yaml.ScalarText(typeVal) {
						case "include":
							includeKeys = keys
						case "exclude":
							excludeKeys = keys
						}
					}

					if err := ExtendYamlService(services, services, fn, i, replaceName, includeKeys, excludeKeys, true); err != nil {
						return err
					}
				}
			}
		}
	}

	// Scale expansion — expand service with scale:N into service1..serviceN
	if services != nil {
		newValues := make([]*yaml.MappingValueNode, 0, len(services.Values))
		for _, svcPair := range services.Values {
			keyNode := svcPair.Key
			valNode := svcPair.Value

			var scaleVal yaml.Node
			if svcMap, ok := yaml.Resolve(ctx, valNode).(*yaml.MappingNode); ok {
				if idx, sv := yaml.FindValue(ctx, svcMap, "scale"); sv != nil {
					yaml.RemovePair(svcMap, idx)
					scaleVal = sv
				}
			}

			if scaleVal == nil || scaleVal.Type() == yaml.NullType || yaml.ScalarText(scaleVal) == "" || yaml.ScalarText(scaleVal) == "false" {
				newValues = append(newValues, svcPair)
				continue
			}

			scale, err := strconv.Atoi(yaml.ScalarText(scaleVal))
			if err != nil || scale <= 1 {
				newValues = append(newValues, svcPair)
				continue
			}

			for j := 1; j <= scale; j++ {
				data, err := yaml.Dump(valNode)
				if err != nil {
					return err
				}
				copyNode, err := yaml.Load(data)
				if err != nil {
					return err
				}
				copyDoc, _ := copyNode.(*yaml.DocumentNode)
				var copyBody yaml.Node
				if copyDoc != nil {
					copyBody = copyDoc.Body
				}

				// copyBody is a fresh reparse of valNode's own dumped text, so
				// its children already sit at the correct depth relative to
				// keyNode's column - only the newly synthesized key (built at
				// column 1) needs realigning to slot in at the same column
				// keyNode itself already occupies.
				newKey := yaml.NewString(fmt.Sprintf("%s%d", yaml.ScalarText(keyNode), j))
				alignedKey, _ := yaml.AlignColumn(newKey, yaml.Column(keyNode)).(*yaml.StringNode)
				newValues = append(newValues, yaml.NewPair(alignedKey, copyBody))
			}
		}
		services.Values = newValues
	}

	addAnchors(node)

	return nil
}

func ExtendYamlService(services, exServices, fn yaml.Node, replaceIndex int, replaceName string, includeKeys, excludeKeys []string, stripPortsScale bool) error {
	ctx := yaml.NewCtx()
	ctx.Index(services)
	if exServices != services {
		ctx.Index(exServices)
	}

	svcMap, ok := yaml.Resolve(ctx, services).(*yaml.MappingNode)
	if !ok {
		return fmt.Errorf("extend: services node is not a mapping")
	}
	if replaceIndex < 0 || replaceIndex >= len(svcMap.Values) {
		return fmt.Errorf("extend: replace index %d out of range", replaceIndex)
	}
	node := svcMap.Values[replaceIndex].Value

	_, exVal := yaml.FindValue(ctx, exServices, replaceName)
	if exVal == nil {
		return fmt.Errorf("extend: base service %q not found", replaceName)
	}

	// filterNodeByKeys always returns a fresh copy (even with empty include/exclude
	// filters), so extendNode below never mutates the base service's original node.
	ex := filterNodeByKeys(ctx, exVal, includeKeys, excludeKeys)
	exMap, _ := ex.(*yaml.MappingNode)

	// extend (same file) does not inherit ports or scale from the base service
	if stripPortsScale && exMap != nil {
		for _, key := range []string{"ports", "scale"} {
			if idx, n := yaml.FindValue(ctx, exMap, key); n != nil {
				yaml.RemovePair(exMap, idx)
			}
		}
	}

	if fn != nil && yaml.ShortTag(fn) == "!!js/function" {
		lData := &yaml.NodeManager{Node: node, Ctx: ctx}
		lJson, err := lData.JSON()
		if err != nil {
			return err
		}

		sData := &yaml.NodeManager{Node: ex, Ctx: ctx}
		sJson, err := sData.JSON()
		if err != nil {
			return err
		}

		jsonStr := `{ "local": %LOCAL%, "service": %SERVICE% }`
		jsonStr = strings.Replace(jsonStr, "%LOCAL%", string(lJson), 1)
		jsonStr = strings.Replace(jsonStr, "%SERVICE%", string(sJson), 1)

		output, err := RunJS(yaml.ScalarText(fn), jsonStr)
		if err != nil {
			return err
		}

		if len(output) > 0 {
			ymlData, err := JsonToYaml(output)
			if err != nil {
				return err
			}
			ymlNode, err := yaml.Load([]byte(ymlData))
			if err != nil {
				return err
			}
			ctx.Index(ymlNode)

			if nodeMap, ok := yaml.Resolve(ctx, node).(*yaml.MappingNode); ok {
				extendNode(ctx, nodeMap, ymlNode)
			}
		}
	}

	if exMap != nil {
		extendNode(ctx, exMap, node)
	}

	yaml.ReplacePairValue(svcMap, replaceIndex, ex)

	return nil
}

func JsonToYaml(data string) (string, error) {
	var m interface{}

	err := json.Unmarshal([]byte(data), &m)
	if err != nil {
		return "", err
	}

	bytes, err := yaml.Marshal(&m)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}
