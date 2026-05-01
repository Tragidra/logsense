package cluster

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Tragidra/logstruct/model"
)

// LogGroup is a mined log template with aggregated statistics.
type LogGroup struct {
	ID         string
	TokensTmpl []string
	Count      int64
	FirstSeen  time.Time
	LastSeen   time.Time
	Services   map[string]int
	Levels     map[model.Level]int64
	Examples   *Reservoir
}

type node struct {
	children map[string]*node
	groups   []*LogGroup
	depth    int
}

// Tree is the fixed-depth Drain parse tree and all methods are safe for concurrent use
type Tree struct {
	mu           sync.Mutex
	root         *node
	maxDepth     int
	maxChildren  int
	simThreshold float64
	maxTemplates int
}

// NewTree returns a ready Tree with the given parameters, defaults:
// maxDepth=3, maxChildren=100, maxTemplates=10, simThreshold=0.4.
func NewTree(maxDepth, maxChildren, maxTemplates int, simThreshold float64) *Tree {
	return &Tree{
		root:         &node{children: make(map[string]*node)},
		maxDepth:     maxDepth,
		maxChildren:  maxChildren,
		simThreshold: simThreshold,
		maxTemplates: maxTemplates,
	}
}

// Insert classifies e into an existing LogGroup or creates a new one.
func (t *Tree) Insert(e model.LogEvent) *LogGroup {
	tokens := Tokenize(e.Message)
	if len(tokens) == 0 {
		tokens = []string{"<empty>"}
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	n := t.root

	n = getOrCreate(n, bucketKey(len(tokens)), t.maxChildren)

	for d := 1; d < t.maxDepth && d <= len(tokens); d++ {
		tok := tokens[d-1]
		if hasDigit(tok) || looksParameterLike(tok) {
			tok = "<*>"
		}
		n = getOrCreate(n, tok, t.maxChildren)
	}

	if best := findBest(n.groups, tokens, t.simThreshold); best != nil {
		mergeTemplate(best, tokens)
		best.Count++
		best.LastSeen = e.Timestamp
		if e.Service != "" {
			best.Services[e.Service]++
		}
		best.Levels[e.Level]++
		best.Examples.Add(e.Raw)
		return best
	}

	if len(n.groups) >= t.maxTemplates {
		evictLRU(n)
	}

	ts := e.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	g := &LogGroup{
		ID:         model.NewID(),
		TokensTmpl: append([]string(nil), tokens...),
		Count:      1,
		FirstSeen:  ts,
		LastSeen:   ts,
		Services:   make(map[string]int),
		Levels:     make(map[model.Level]int64),
		Examples:   NewReservoir(5),
	}
	if e.Service != "" {
		g.Services[e.Service] = 1
	}
	g.Levels[e.Level] = 1
	g.Examples.Add(e.Raw)
	n.groups = append(n.groups, g)
	return g
}

// AllGroups returns a snapshot of every LogGroup in the tree.
func (t *Tree) AllGroups() []*LogGroup {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []*LogGroup
	var walk func(*node)
	walk = func(n *node) {
		out = append(out, n.groups...)
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(t.root)
	return out
}

func (t *Tree) Prune(olderThan time.Duration) {
	cutoff := time.Now().Add(-olderThan)
	t.mu.Lock()
	defer t.mu.Unlock()
	pruneNode(t.root, cutoff)
}

// Save serializes the tree to path using gob, write is atomic (temp + rename).
func (t *Tree) Save(path string) error {
	t.mu.Lock()
	snap := buildSnap(t)
	t.mu.Unlock()

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("cluster: save: mkdir: %w", err)
		}
	}

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("cluster: save: %w", err)
	}
	if err := gob.NewEncoder(f).Encode(snap); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("cluster: save encode: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("cluster: save close: %w", err)
	}
	return os.Rename(tmp, path)
}

// Load deserializes a tree snapshot from path, Returns os.ErrNotExist if the file is absent;
// (other errors indicate corruption)
func (t *Tree) Load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var snap treeSnap
	if err := gob.NewDecoder(f).Decode(&snap); err != nil {
		return fmt.Errorf("cluster: load decode: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	restoreSnap(t, &snap)
	return nil
}

func bucketKey(n int) string {
	for _, k := range []int{4, 8, 16, 32, 64} {
		if n <= k {
			return fmt.Sprintf("%d", k)
		}
	}
	return "128"
}

// getOrCreate returns the child of n keyed by key, creating it if absent.
func getOrCreate(n *node, key string, maxChildren int) *node {
	if c, ok := n.children[key]; ok {
		return c
	}
	if len(n.children) >= maxChildren {
		key = "<*>"
		if c, ok := n.children[key]; ok {
			return c
		}
	}
	c := &node{children: make(map[string]*node), depth: n.depth + 1}
	n.children[key] = c
	return c
}

func findBest(groups []*LogGroup, tokens []string, threshold float64) *LogGroup {
	var best *LogGroup
	var bestSim float64
	for _, g := range groups {
		if s := similarity(g.TokensTmpl, tokens); s > bestSim {
			bestSim = s
			best = g
		}
	}
	if bestSim >= threshold {
		return best
	}
	return nil
}

func similarity(tmpl, tokens []string) float64 {
	if len(tmpl) != len(tokens) || len(tmpl) == 0 {
		return 0
	}
	matches := 0
	for i := range tmpl {
		if tmpl[i] == "<*>" || tmpl[i] == tokens[i] {
			matches++
		}
	}
	return float64(matches) / float64(len(tmpl))
}

func mergeTemplate(g *LogGroup, tokens []string) {
	for i, t := range g.TokensTmpl {
		if t != "<*>" && t != tokens[i] {
			g.TokensTmpl[i] = "<*>"
		}
	}
}

func evictLRU(n *node) {
	if len(n.groups) == 0 {
		return
	}
	oldest := 0
	for i, g := range n.groups {
		if g.LastSeen.Before(n.groups[oldest].LastSeen) {
			oldest = i
		}
	}
	n.groups = append(n.groups[:oldest], n.groups[oldest+1:]...)
}

func pruneNode(n *node, cutoff time.Time) bool {
	kept := n.groups[:0]
	for _, g := range n.groups {
		if !g.LastSeen.Before(cutoff) {
			kept = append(kept, g)
		}
	}
	n.groups = kept
	for k, c := range n.children {
		if pruneNode(c, cutoff) {
			delete(n.children, k)
		}
	}
	return len(n.groups) == 0 && len(n.children) == 0
}

type treeSnap struct {
	Root         *nodeSnap
	MaxDepth     int
	MaxChildren  int
	SimThreshold float64
	MaxTemplates int
}

type nodeSnap struct {
	Children map[string]*nodeSnap
	Groups   []*groupSnap
}

type groupSnap struct {
	ID         string
	TokensTmpl []string
	Count      int64
	FirstSeen  time.Time
	LastSeen   time.Time
	Services   map[string]int
	Levels     map[int]int64
	Examples   []string
}

func buildSnap(t *Tree) *treeSnap {
	return &treeSnap{
		Root:         nodeToSnap(t.root),
		MaxDepth:     t.maxDepth,
		MaxChildren:  t.maxChildren,
		SimThreshold: t.simThreshold,
		MaxTemplates: t.maxTemplates,
	}
}

func nodeToSnap(n *node) *nodeSnap {
	s := &nodeSnap{
		Children: make(map[string]*nodeSnap, len(n.children)),
		Groups:   make([]*groupSnap, 0, len(n.groups)),
	}
	for k, c := range n.children {
		s.Children[k] = nodeToSnap(c)
	}
	for _, g := range n.groups {
		lvl := make(map[int]int64, len(g.Levels))
		for l, cnt := range g.Levels {
			lvl[int(l)] = cnt
		}
		s.Groups = append(s.Groups, &groupSnap{
			ID: g.ID, TokensTmpl: g.TokensTmpl, Count: g.Count,
			FirstSeen: g.FirstSeen, LastSeen: g.LastSeen,
			Services: g.Services, Levels: lvl, Examples: g.Examples.Items(),
		})
	}
	return s
}

func restoreSnap(t *Tree, snap *treeSnap) {
	t.root = snapToNode(snap.Root, 0)
	t.maxDepth = snap.MaxDepth
	t.maxChildren = snap.MaxChildren
	t.simThreshold = snap.SimThreshold
	t.maxTemplates = snap.MaxTemplates
}

func snapToNode(s *nodeSnap, depth int) *node {
	n := &node{children: make(map[string]*node, len(s.Children)), depth: depth}
	for k, c := range s.Children {
		n.children[k] = snapToNode(c, depth+1)
	}
	for _, gs := range s.Groups {
		lvl := make(map[model.Level]int64, len(gs.Levels))
		for l, cnt := range gs.Levels {
			lvl[model.Level(l)] = cnt
		}
		res := NewReservoir(5)
		for _, ex := range gs.Examples {
			res.Add(ex)
		}
		n.groups = append(n.groups, &LogGroup{
			ID: gs.ID, TokensTmpl: gs.TokensTmpl, Count: gs.Count,
			FirstSeen: gs.FirstSeen, LastSeen: gs.LastSeen,
			Services: gs.Services, Levels: lvl, Examples: res,
		})
	}
	return n
}
