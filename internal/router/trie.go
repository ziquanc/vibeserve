package router

import "strings"

// RouteEntry represents a registered route.
type RouteEntry struct {
	Method string
	Path   string
	Script string
}

// trieNode is a node in the routing trie.
type trieNode struct {
	children map[string]*trieNode
	param    string            // non-empty for :param segments (stores param name without ":")
	scripts  map[string]string // method → script name
}

// Trie is a prefix tree for URL routing.
type Trie struct {
	root *trieNode
}

// NewTrie creates an empty Trie.
func NewTrie() *Trie {
	return &Trie{root: &trieNode{children: make(map[string]*trieNode)}}
}

// segments splits a URL path into non-empty segments.
func segments(path string) []string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Insert registers a route in the trie.
func (t *Trie) Insert(method, path, script string) {
	node := t.root
	for _, seg := range segments(path) {
		if strings.HasPrefix(seg, ":") {
			// Parameter segment — stored under the key ":"
			if _, ok := node.children[":"]; !ok {
				node.children[":"] = &trieNode{
					children: make(map[string]*trieNode),
					param:    seg[1:], // strip the ":"
				}
			}
			node = node.children[":"]
		} else {
			if _, ok := node.children[seg]; !ok {
				node.children[seg] = &trieNode{children: make(map[string]*trieNode)}
			}
			node = node.children[seg]
		}
	}
	if node.scripts == nil {
		node.scripts = make(map[string]string)
	}
	node.scripts[strings.ToUpper(method)] = script
}

// Search looks up a route and returns the script name, captured path params, and whether it was found.
func (t *Trie) Search(method, path string) (script string, params map[string]string, found bool) {
	params = make(map[string]string)
	node := t.root
	for _, seg := range segments(path) {
		if child, ok := node.children[seg]; ok {
			// Exact match takes priority
			node = child
		} else if child, ok := node.children[":"]; ok {
			// Parameter match
			params[child.param] = seg
			node = child
		} else {
			return "", nil, false
		}
	}
	if node.scripts == nil {
		return "", nil, false
	}
	s, ok := node.scripts[strings.ToUpper(method)]
	if !ok {
		return "", nil, false
	}
	return s, params, true
}

// Remove deletes a method+path route from the trie.
func (t *Trie) Remove(method, path string) {
	node := t.root
	for _, seg := range segments(path) {
		if strings.HasPrefix(seg, ":") {
			child, ok := node.children[":"]
			if !ok {
				return
			}
			node = child
		} else {
			child, ok := node.children[seg]
			if !ok {
				return
			}
			node = child
		}
	}
	if node.scripts != nil {
		delete(node.scripts, strings.ToUpper(method))
	}
}

// Routes returns all registered routes.
func (t *Trie) Routes() []RouteEntry {
	var result []RouteEntry
	collectRoutes(t.root, "", &result)
	return result
}

// collectRoutes recursively walks the trie and collects all RouteEntry values.
func collectRoutes(node *trieNode, pathSoFar string, result *[]RouteEntry) {
	for method, script := range node.scripts {
		path := pathSoFar
		if path == "" {
			path = "/"
		}
		*result = append(*result, RouteEntry{Method: method, Path: path, Script: script})
	}
	for seg, child := range node.children {
		var nextSeg string
		if seg == ":" {
			nextSeg = pathSoFar + "/:" + child.param
		} else {
			nextSeg = pathSoFar + "/" + seg
		}
		collectRoutes(child, nextSeg, result)
	}
}
