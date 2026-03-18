package gofi

type nodeType uint8

const (
	static nodeType = iota // default
	root
	param
	catchAll
)

type routeData struct {
	pattern  string
	handlers []HandlerFunc
	rules    *schemaRules
	meta     any
}

type node struct {
	path      string
	indices   string
	wildChild bool
	nType     nodeType
	priority  uint32
	children  []*node
	data      *routeData
	maxParams uint8
}

// increments priority of the given child and reorders if necessary
func (n *node) incrementChildPrio(pos int) int {
	cs := n.children
	cs[pos].priority++
	prio := cs[pos].priority

	// Adjust position (move to front)
	newPos := pos
	for newPos > 0 && cs[newPos-1].priority < prio {
		cs[newPos-1], cs[newPos] = cs[newPos], cs[newPos-1]
		newPos--
	}

	// Build new index char string
	if newPos != pos {
		n.indices = n.indices[:newPos] +
			n.indices[pos:pos+1] +
			n.indices[newPos:pos] + n.indices[pos+1:]
	}

	return newPos
}

// addRoute adds a node with the given route path to the tree.
func (n *node) addRoute(path string, data *routeData) {
	fullPath := path
	n.priority++
	numParams := countParams(path)

	if n.maxParams < numParams {
		n.maxParams = numParams
	}

	// Empty tree
	if len(n.path) == 0 && len(n.children) == 0 {
		n.insertChild(path, fullPath, data)
		n.nType = root
		return
	}

walk:
	for {
		// Update maxParams of the current node
		if numParams > n.maxParams {
			n.maxParams = numParams
		}

		// Find the longest common prefix
		i := longestCommonPrefix(path, n.path)

		// Split edge
		if i < len(n.path) {
			child := node{
				path:      n.path[i:],
				wildChild: n.wildChild,
				indices:   n.indices,
				children:  n.children,
				data:      n.data,
				priority:  n.priority - 1,
				maxParams: n.maxParams, // Pass down maxParams
			}

			// Update maxParams for child based on remaining path
			child.maxParams = countParams(child.path) + childMaxParams(child.children)

			n.children = []*node{&child}
			// []byte for proper unicode char extraction
			n.indices = string([]byte{n.path[i]})
			n.path = path[:i]
			n.data = nil
			n.wildChild = false
		}

		// Make new node a child of this node
		if i < len(path) {
			path = path[i:]

			if n.wildChild {
				n = n.children[0]
				n.priority++

				// Update maxParams
				if numParams > n.maxParams {
					n.maxParams = numParams
				}
				numParams-- // We just matched one param

				// Check if the wildcard matches
				if len(path) >= len(n.path) && n.path == path[:len(n.path)] &&

					// Adding a child to a catchAll is not possible
					n.nType != catchAll &&

					// Check for longer wildcard, e.g. :name and :names
					(len(n.path) >= len(path) || path[len(n.path)] == '/') {
					continue walk
				}
				panic("path segment '" + path + "' conflicts with existing wildcard '" + n.path + "' in path '" + fullPath + "'")
			}

			c := path[0]

			// slash after param
			if n.nType == param && c == '/' && len(n.children) == 1 {
				n = n.children[0]
				n.priority++
				continue walk
			}

			// Check if a child with the next path byte exists
			for i, max := 0, len(n.indices); i < max; i++ {
				if c == n.indices[i] {
					i = n.incrementChildPrio(i)
					n = n.children[i]
					continue walk
				}
			}

			// Otherwise insert it
			if c != ':' && c != '*' {
				// []byte for proper unicode char extraction
				n.indices += string([]byte{c})
				child := &node{
					maxParams: numParams,
				}
				n.children = append(n.children, child)
				n.incrementChildPrio(len(n.indices) - 1)
				n = child
			}
			n.insertChild(path, fullPath, data)
			return
		}

		// Default behaviour: the node already exists
		if n.data != nil {
			panic("a route is already registered for path '" + fullPath + "'")
		}
		n.data = data
		return
	}
}

func (n *node) insertChild(path, fullPath string, data *routeData) {
	for {
		// Find prefix until first wildcard
		wildcard, i, valid := findWildcard(path)
		if i < 0 {
			// No wildcard found
			break
		}

		// The wildcard name must solely contain letters and numbers
		if !valid {
			panic("only named parameters and catch-all wildcards are allowed: " + path)
		}

		// Check if the wildcard has a name
		if len(wildcard) < 2 {
			panic("wildcards must be named with a non-empty name in path '" + fullPath + "'")
		}

		if wildcard[0] == ':' { // param
			if i > 0 {
				// Insert prefix before the current wildcard
				n.path = path[:i]
				path = path[i:]
			}

			n.wildChild = true
			child := &node{
				nType: param,
				path:  wildcard,
			}
			n.children = []*node{child}
			n = child
			n.priority++

			if len(wildcard) < len(path) {
				path = path[len(wildcard):]
				child := &node{
					priority: 1,
				}
				n.children = []*node{child}
				n = child
				continue
			}

			// Otherwise we're done. Insert the handler in the new leaf
			n.data = data
			return

		} else if wildcard[0] == '*' { // catchAll
			if i+len(wildcard) != len(path) {
				panic("catch-all routes are only allowed at the end of the path in path '" + fullPath + "'")
			}

			if len(n.path) > 0 && n.path[len(n.path)-1] == '/' {
				panic("catch-all conflicts with existing handle for the path segment root in path '" + fullPath + "'")
			}

			i--
			if path[i] != '/' {
				panic("no / before catch-all in path '" + fullPath + "'")
			}

			n.path = path[:i]

			// First node: catchAll node with empty path
			child := &node{
				wildChild: true,
				nType:     catchAll,
			}
			n.children = []*node{child}
			n.indices = string('/')
			n = child
			n.priority++

			// Second node: node holding the variable
			child = &node{
				path:     path[i:],
				nType:    catchAll,
				data:     data,
				priority: 1,
			}
			n.children = []*node{child}

			return
		}
	}

	// No wildcard found, simply insert the path and handler
	n.path = path
	n.data = data
}

// getValue returns the routeData registered with the given path (key). The values of
// wildcards are saved to a slice.
func (n *node) getValue(path string, params func() *Params) (data *routeData, tsr bool) {
walk:
	for {
		prefix := n.path
		if len(path) > len(prefix) {
			if path[:len(prefix)] == prefix {
				path = path[len(prefix):]

				if !n.wildChild {
					c := path[0]
					for i, max := 0, len(n.indices); i < max; i++ {
						if c == n.indices[i] {
							n = n.children[i]
							continue walk
						}
					}

					// Nothing found.
					// We can recommend to redirect to the same URL without a
					// trailing slash if a leaf exists for that path.
					tsr = (path == "/" && n.data != nil)
					return
				}

				// handle wildcard child
				n = n.children[0]
				switch n.nType {
				case param:
					end := 0
					for end < len(path) && path[end] != '/' {
						end++
					}

					if params != nil {
						p := params()
						*p = append(*p, Param{
							Key:   n.path[1:],
							Value: path[:end],
						})
					}

					if end < len(path) {
						if len(n.children) > 0 {
							path = path[end:]
							n = n.children[0]
							continue walk
						}
						// ... but we can't
						tsr = (len(path) == end+1)
						return
					}

					if data = n.data; data != nil {
						return
					} else if len(n.children) == 1 {
						// No handle found. Check if a handle for this path + a
						// trailing slash exists for TSR recommendation
						n = n.children[0]
						tsr = (n.path == "/" && n.data != nil)
					}

					return

				case catchAll:
					if params != nil {
						p := params()
						*p = append(*p, Param{
							Key:   n.path[2:],
							Value: path[1:],
						})
					}

					data = n.data
					return

				default:
					panic("invalid node type")
				}
			}
		} else if path == prefix {
			// We should have reached the node containing the handle.
			// Check if this node has a handle registered.
			if data = n.data; data != nil {
				return
			}

			// If there is no handle for this route, but this route has a
			// wildcard child, there must be a handle for this path with an
			// additional trailing slash
			if path == "/" && n.wildChild && n.nType != root {
				tsr = true
				return
			}

			// No handle found. Check if a handle for this path + a
			// trailing slash exists for trailing slash recommendation
			for i, max := 0, len(n.indices); i < max; i++ {
				if n.indices[i] == '/' {
					n = n.children[i]
					tsr = (len(n.path) == 1 && n.data != nil) ||
						(n.nType == catchAll && n.children[0].data != nil)
					return
				}
			}

			return
		}

		// Nothing found. We can recommend to redirect to the same URL with an
		// extra trailing slash if a leaf exists for that path
		tsr = (path == "/") ||
			(len(prefix) == len(path)+1 && prefix[len(path)] == '/' &&
				path == prefix[:len(prefix)-1] && n.data != nil)
		return
	}
}

func min(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

func longestCommonPrefix(a, b string) int {
	i := 0
	max := min(len(a), len(b))
	for i < max && a[i] == b[i] {
		i++
	}
	return i
}

func findWildcard(path string) (wildcard string, i int, valid bool) {
	for start, c := range []byte(path) {
		if c != ':' && c != '*' {
			continue
		}

		valid = true
		for end, c := range []byte(path[start+1:]) {
			switch c {
			case '/':
				return path[start : start+1+end], start, valid
			case ':', '*':
				valid = false
			}
		}
		return path[start:], start, valid
	}
	return "", -1, false
}

func countParams(path string) uint8 {
	var n uint
	for i := 0; i < len(path); i++ {
		if path[i] == ':' || path[i] == '*' {
			n++
		}
	}
	if n >= 255 {
		return 255
	}
	return uint8(n)
}

func childMaxParams(children []*node) uint8 {
	var max uint8
	for _, child := range children {
		if child.maxParams > max {
			max = child.maxParams
		}
	}
	return max
}
