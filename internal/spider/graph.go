package spider

import (
	"fmt"
	"sync"
)

type Node struct {
	Login       string
	Name        string
	AvatarURL   string
	Followers   int
	Following   int
	PublicRepos int
	Company     string
	Location    string
	Bio         string
	Depth       int
}

type Edge struct {
	Source string
	Target string
	Type   string
	Weight int
	Repo   string
}

func edgeKey(source, target, edgeType string) string {
	return fmt.Sprintf("%s|%s|%s", source, target, edgeType)
}

type Graph struct {
	Nodes map[string]*Node
	Edges map[string]*Edge
	mu    sync.RWMutex
}

func NewGraph() *Graph {
	return &Graph{
		Nodes: make(map[string]*Node),
		Edges: make(map[string]*Edge),
	}
}

func (g *Graph) AddNode(node *Node) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.Nodes[node.Login]; exists {
		return false
	}
	g.Nodes[node.Login] = node
	return true
}

func (g *Graph) HasNode(login string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, exists := g.Nodes[login]
	return exists
}

func (g *Graph) NodeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.Nodes)
}

func (g *Graph) AddEdge(source, target, edgeType string, repo string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	key := edgeKey(source, target, edgeType)
	if existing, ok := g.Edges[key]; ok {
		existing.Weight++
		return
	}
	g.Edges[key] = &Edge{
		Source: source,
		Target: target,
		Type:   edgeType,
		Weight: 1,
		Repo:   repo,
	}
}

func (g *Graph) EdgeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.Edges)
}
