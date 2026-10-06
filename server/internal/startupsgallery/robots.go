package startupsgallery

import (
	"bufio"
	"bytes"
	"strings"
)

// robotsRules are the Allow and Disallow lines of robots.txt that apply to
// the hub: the group naming its product token, or else the "*" group.
type robotsRules struct {
	allow    []string
	disallow []string
}

// readRobots reads the rules for the agent from a robots.txt file.
func readRobots(file []byte, agent string) robotsRules {
	agent = strings.ToLower(agent)
	var named, anyAgent robotsRules
	var foundNamed bool
	var groupAgents []string
	inRules := false
	scanner := bufio.NewScanner(bytes.NewReader(file))
	for scanner.Scan() {
		line, _, _ := strings.Cut(scanner.Text(), "#")
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key, value = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value)
		switch key {
		case "user-agent":
			if inRules {
				groupAgents, inRules = nil, false
			}
			groupAgents = append(groupAgents, strings.ToLower(value))
		case "allow", "disallow":
			inRules = true
			for _, groupAgent := range groupAgents {
				var rules *robotsRules
				switch {
				case groupAgent == "*":
					rules = &anyAgent
				case groupAgent != "" && strings.Contains(agent, groupAgent):
					rules, foundNamed = &named, true
				default:
					continue
				}
				if value == "" {
					continue
				}
				if key == "allow" {
					rules.allow = append(rules.allow, value)
				} else {
					rules.disallow = append(rules.disallow, value)
				}
			}
		}
	}
	if foundNamed {
		return named
	}
	return anyAgent
}

// allows reports whether the rules let the hub read a path: the longest
// matching rule decides, and Allow wins a tie.
func (rules robotsRules) allows(path string) bool {
	longestAllow, longestDisallow := -1, -1
	for _, pattern := range rules.allow {
		if matchesRobotsPattern(pattern, path) && len(pattern) > longestAllow {
			longestAllow = len(pattern)
		}
	}
	for _, pattern := range rules.disallow {
		if matchesRobotsPattern(pattern, path) && len(pattern) > longestDisallow {
			longestDisallow = len(pattern)
		}
	}
	return longestAllow >= longestDisallow
}

// matchesRobotsPattern matches a path against a rule's pattern, where "*"
// stands for any characters and a final "$" anchors the end.
func matchesRobotsPattern(pattern, path string) bool {
	anchored := strings.HasSuffix(pattern, "$")
	pattern = strings.TrimSuffix(pattern, "$")
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(path, parts[0]) {
		return false
	}
	rest := path[len(parts[0]):]
	for index, part := range parts[1:] {
		if index == len(parts)-2 && anchored {
			return strings.HasSuffix(rest, part)
		}
		found := strings.Index(rest, part)
		if found < 0 {
			return false
		}
		rest = rest[found+len(part):]
	}
	return !anchored || rest == ""
}
